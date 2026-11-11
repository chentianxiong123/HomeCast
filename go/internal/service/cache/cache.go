// Package cache 音频缓存域：落盘缓存播放过的歌曲（LRU 上限淘汰）。
//
// 全链路：入口 internal/api/cache.go；前端 player.js 用 /api/v1/cache/* 接口；
// 表：audio_cache（本包 CacheSchema）+ settings（kv，存缓存上限）。
//
// 设计对齐旧 Python 版 bvid_cache.py：
//   - 缓存标识 bvid（解决 B站 URL 每次不同）
//   - 音频文件落盘 dataDir/cache/audio/{bvid}.m4s（不塞 SQLite blob——大文件会撑爆 DB/WAL）
//   - SQLite 只存元数据（size/quality/last_used）做 LRU 淘汰 + 清除
//   - 再次播放秒开：Stream 先查缓存，命中直接 serve 文件
//
// 缓存触发：播放超 60s 且未缓存 → 后台拉全量落盘（对齐前端 cache.js 原逻辑，去 IndexedDB）。
// 清除两档：全清 / 只清不在 favs 表（未收藏）的歌。
// 上限：后端持有并持久化（settings kv），前端经 /api/v1/cache/limit 读写。
package cache

import (
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// CacheTable audio_cache 表名
const CacheTable = "audio_cache"

// CacheSchema audio_cache 表定义（表结构归业务域；建表执行在 store.Migrate 统一跑）
const CacheSchema = `CREATE TABLE IF NOT EXISTS audio_cache (
	bvid TEXT PRIMARY KEY, size INTEGER DEFAULT 0, quality INTEGER DEFAULT 0,
	last_used INTEGER DEFAULT 0, created_at INTEGER DEFAULT 0)`

// SettingsSchema settings kv 表（存缓存上限等）
const SettingsSchema = `CREATE TABLE IF NOT EXISTS settings (
	key TEXT PRIMARY KEY, value TEXT)`

// DefaultLimitMB 默认缓存上限（MB）；0=关闭
const DefaultLimitMB = 500

// limitKey settings kv 键
const limitKey = "cache_limit_mb"

// CacheService 音频缓存（SQLite 元数据 + 磁盘文件）
type CacheService struct {
	mu       sync.Mutex
	db       *sql.DB
	audioDir string // dataDir/cache/audio
	limitMB  int    // 后端持有并持久化（0=关）
}

// NewCacheService 构造。audioDir 为缓存音频目录；limitMB 初值（<0 用默认）。
func NewCacheService(db *sql.DB, audioDir string, limitMB int) *CacheService {
	s := &CacheService{db: db, audioDir: audioDir, limitMB: DefaultLimitMB}
	if limitMB >= 0 {
		s.limitMB = limitMB
	} else if v, err := s.loadLimit(); err == nil {
		s.limitMB = v
	}
	return s
}

// loadLimit 从 settings 读上限
func (s *CacheService) loadLimit() (int, error) {
	var v string
	if err := s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, limitKey).Scan(&v); err != nil {
		return 0, err
	}
	n := 0
	for _, c := range v {
		if c < '0' || c > '9' {
			return DefaultLimitMB, nil
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

// LimitMB 当前上限（MB，0=关）
func (s *CacheService) LimitMB() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.limitMB
}

// SetLimitMB 设置上限并持久化（0=关）
func (s *CacheService) SetLimitMB(v int) {
	if v < 0 {
		v = 0
	}
	s.mu.Lock()
	s.limitMB = v
	s.db.Exec(`INSERT INTO settings (key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=?`,
		limitKey, itoa(v), itoa(v))
	s.evictLocked() // 降低上限可能触发立即淘汰
	s.mu.Unlock()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 12)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}

// filePath 音频缓存文件路径
func (s *CacheService) filePath(bvid string) string {
	return filepath.Join(s.audioDir, bvid+".m4s")
}

// Get 取缓存元数据（命中则刷新 last_used=LRU 触碰）；文件不存在返回 false
func (s *CacheService) Get(bvid string) (size int64, quality int, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.filePath(bvid)
	fi, err := os.Stat(path)
	if err != nil || fi.Size() == 0 {
		return 0, 0, false
	}
	var q int
	dbSize := fi.Size()
	if err := s.db.QueryRow(`SELECT size, quality FROM audio_cache WHERE bvid=?`, bvid).Scan(&dbSize, &q); err == nil {
		if dbSize > fi.Size() {
			// 文件比元数据小 → 下载中断/截断，serve 后浏览器 seek 到超出实际长度位置会 416
			// → 音频跳末尾触发 ended 误切歌。删记录返回未命中，让 stream 重拉完整文件。
			s.db.Exec(`DELETE FROM audio_cache WHERE bvid=?`, bvid)
			os.Remove(path)
			return 0, 0, false
		}
		if dbSize != fi.Size() { // 元数据过期（文件变大）→ 按实际
			s.db.Exec(`UPDATE audio_cache SET size=? WHERE bvid=?`, fi.Size(), bvid)
		}
	}
	now := time.Now().UnixMilli()
	s.db.Exec(`INSERT INTO audio_cache (bvid,size,quality,last_used) VALUES (?,?,?,?)
		ON CONFLICT(bvid) DO UPDATE SET size=?, quality=?, last_used=?`,
		bvid, fi.Size(), q, now, fi.Size(), q, now)
	return fi.Size(), q, true
}

// FilePath 暴露给 handler 直接 serve 文件（命中时）
func (s *CacheService) FilePath(bvid string) string { return s.filePath(bvid) }

// Put 写缓存：data 非空则先落盘文件；再写元数据（data 为空=文件已存在，仅写元数据）+ LRU 淘汰
func (s *CacheService) Put(bvid string, quality int, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.audioDir, 0o755); err != nil {
		return err
	}
	path := s.filePath(bvid)
	size := int64(len(data))
	if len(data) > 0 {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
	} else if fi, err := os.Stat(path); err == nil {
		size = fi.Size()
	} else {
		return err
	}
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(`INSERT INTO audio_cache (bvid,size,quality,last_used,created_at)
		VALUES (?,?,?,?,?) ON CONFLICT(bvid) DO UPDATE SET size=?, quality=?, last_used=?`,
		bvid, size, quality, now, now, size, quality, now)
	if err != nil {
		return err
	}
	s.evictLocked()
	return nil
}

// evictLocked LRU 淘汰：总量超上限时删最旧 last_used，直到达标（需持锁）
func (s *CacheService) evictLocked() {
	if s.limitMB <= 0 {
		return // 关闭缓存
	}
	limitBytes := int64(s.limitMB) * 1024 * 1024
	rows, err := s.db.Query(`SELECT bvid, size, last_used FROM audio_cache`)
	if err != nil {
		return
	}
	type ent struct {
		bvid     string
		size     int64
		lastUsed int64
	}
	var ents []ent
	var total int64
	for rows.Next() {
		var e ent
		if rows.Scan(&e.bvid, &e.size, &e.lastUsed) == nil {
			ents = append(ents, e)
			total += e.size
		}
	}
	rows.Close()
	if total <= limitBytes {
		return
	}
	sort.Slice(ents, func(i, j int) bool { return ents[i].lastUsed < ents[j].lastUsed }) // 最旧在前
	for _, e := range ents {
		if total <= limitBytes {
			break
		}
		os.Remove(s.filePath(e.bvid))
		s.db.Exec(`DELETE FROM audio_cache WHERE bvid=?`, e.bvid)
		total -= e.size
	}
}

// Size 已用大小（MB，两位小数）
func (s *CacheService) Size() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	var total int64
	rows, err := s.db.Query(`SELECT size FROM audio_cache`)
	if err != nil {
		return 0
	}
	for rows.Next() {
		var sz int64
		if rows.Scan(&sz) == nil {
			total += sz
		}
	}
	rows.Close()
	mb := float64(total) / 1024 / 1024
	// 保留两位小数（前端直接展示，免去长浮点尾巴）
	return float64(int(mb*100+0.5)) / 100
}

// Count 缓存条目数
func (s *CacheService) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM audio_cache`).Scan(&n)
	return n
}

// ClearAll 清空全部缓存（文件 + 元数据）
func (s *CacheService) ClearAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	os.RemoveAll(s.audioDir)
	os.MkdirAll(s.audioDir, 0o755)
	s.db.Exec(`DELETE FROM audio_cache`)
}

// StreamAndCache 边写 w（响应流）边落盘缓存（temp→rename，防半截文件）；
// 小流（<64KB）不缓存。供 Stream handler 未命中时调用。
// StreamAndCache 边转发给浏览器边 tee 落盘。contentLength 是上游声明的完整音频字节数，
// 只有 n == contentLength（完整下载）才缓存——下载中断（浏览器 seek/切歌断连）会产生截断文件，
// 截断文件被 serve 后浏览器 seek 超范围→416→跳末尾→误切歌。故不完整一律丢弃，不污染缓存。
func (s *CacheService) StreamAndCache(w io.Writer, bvid string, quality int, body io.Reader, contentLength int64) {
	tmp := s.filePath(bvid) + ".tmp"
	if err := os.MkdirAll(s.audioDir, 0o755); err != nil {
		_, _ = io.Copy(w, body)
		return
	}
	f, err := os.Create(tmp)
	if err != nil {
		_, _ = io.Copy(w, body)
		return
	}
	n, _ := io.Copy(io.MultiWriter(w, f), body)
	f.Close()
	// 只有完整下载（字节数 == 上游 Content-Length）才缓存；不完整（截断）直接丢弃
	if contentLength > 0 && n == contentLength {
		os.Rename(tmp, s.filePath(bvid))
		s.Put(bvid, quality, nil) // 已落盘，只写元数据 + 淘汰（Put 会读文件大小）
	} else {
		os.Remove(tmp)
	}
}

// ClearUnfav 只清不在 favs 表（未收藏）的歌。favTable 为收藏表名（跨域引用，避免循环依赖）
func (s *CacheService) ClearUnfav(favTable string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`SELECT bvid, size FROM audio_cache
		WHERE bvid NOT IN (SELECT bvid FROM ` + favTable + `)`)
	if err != nil {
		return
	}
	type row struct {
		bvid string
		size int64
	}
	var del []row
	for rows.Next() {
		var bvid string
		var size int64
		if rows.Scan(&bvid, &size) == nil {
			del = append(del, row{bvid, size})
		}
	}
	rows.Close()
	for _, d := range del {
		os.Remove(s.filePath(d.bvid))
		s.db.Exec(`DELETE FROM audio_cache WHERE bvid=?`, d.bvid)
	}
}
