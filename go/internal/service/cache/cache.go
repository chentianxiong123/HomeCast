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
	"hash/fnv"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"homecast/internal/service/conv"
)

// CacheTable audio_cache 表名
const CacheTable = "audio_cache"

// CacheSchema audio_cache 表定义（表结构归业务域；建表执行在 store.Migrate 统一跑）
// hash 列：落盘时记文件头 4KB 的 FNV-1a hash（低31位），Get 时校验——文件损坏（非截断）也能检测，
// 命中后 seek 超范围/花屏崩坏这类诡异问题直接删缓存重拉。旧表无 hash 列由 ensureHashColumn 幂等 ALTER。
const CacheSchema = `CREATE TABLE IF NOT EXISTS audio_cache (
	bvid TEXT PRIMARY KEY, size INTEGER DEFAULT 0, quality INTEGER DEFAULT 0,
	hash INTEGER DEFAULT 0,
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
	} else if v, err := conv.LoadLimit(s.db, limitKey, DefaultLimitMB); err == nil {
		s.limitMB = v
	}
	s.ensureHashColumn()
	s.cleanupTmp()
	return s
}

// cleanupTmp 启动时清 .tmp 残留（上次进程被杀，StreamAndCache 未走完 Remove(tmp) 的孤儿）
func (s *CacheService) cleanupTmp() {
	entries, err := os.ReadDir(s.audioDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			os.Remove(filepath.Join(s.audioDir, e.Name()))
		}
	}
}

// ensureHashColumn 幂等加 hash 列（旧库 ALTER，新库建表已含；每次启动调一次，开销可忽略）
func (s *CacheService) ensureHashColumn() {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.Query(`PRAGMA table_info(audio_cache)`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt any
		if rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk) == nil && name == "hash" {
			return
		}
	}
	s.db.Exec(`ALTER TABLE audio_cache ADD COLUMN hash INTEGER DEFAULT 0`)
}

// headHash 文件头 4KB 的 FNV-1a hash（低31位；读取失败返回 0 视为未知，不拦截）
func headHash(path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	buf := make([]byte, 4096)
	n, _ := io.ReadFull(f, buf)
	hh := fnv.New64a()
	hh.Write(buf[:n])
	return int64(hh.Sum64() & 0x7fffffff), nil
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
		limitKey, conv.Itoa(v), conv.Itoa(v))
	s.evictLocked() // 降低上限可能触发立即淘汰
	s.mu.Unlock()
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
	var q, dbHash, lastUsed int64
	dbSize := fi.Size()
	if err := s.db.QueryRow(`SELECT size, quality, hash, last_used FROM audio_cache WHERE bvid=?`, bvid).Scan(&dbSize, &q, &dbHash, &lastUsed); err == nil {
		if dbSize > fi.Size() {
			// 文件比元数据小 → 下载中断/截断，serve 后浏览器 seek 到超出实际长度位置会 416
			// → 音频跳末尾触发 ended 误切歌。删记录返回未命中，让 stream 重拉完整文件。
			s.db.Exec(`DELETE FROM audio_cache WHERE bvid=?`, bvid)
			os.Remove(path)
			return 0, 0, false
		}
		// headHash 校验：落盘写过 hash 且现文件头对不上 → 文件损坏，删缓存重拉
		if dbHash != 0 {
			cur, err := headHash(path)
			if err == nil && cur != dbHash {
				s.db.Exec(`DELETE FROM audio_cache WHERE bvid=?`, bvid)
				os.Remove(path)
				return 0, 0, false
			}
		}
		if dbSize != fi.Size() { // 元数据过期（文件变大）→ 按实际
			s.db.Exec(`UPDATE audio_cache SET size=? WHERE bvid=?`, fi.Size(), bvid)
		}
		// LRU 触碰降频（review 性能 #4：GET 命中每次都写库是写放大——206 分片高频
		// 请求每秒多次触碰。只在距上次触碰 ≥60s 才写 last_used，淘汰粒度秒级足够）
		now := time.Now().UnixMilli()
		if now-lastUsed >= 60000 {
			s.db.Exec(`UPDATE audio_cache SET size=?, last_used=? WHERE bvid=?`, fi.Size(), now, bvid)
		}
	} else {
		// 文件在但元数据缺（旧库/异常）→ 补插，保持淘汰表完整
		s.db.Exec(`INSERT INTO audio_cache (bvid,size,quality,hash,last_used) VALUES (?,?,?,?,?)`,
			bvid, fi.Size(), q, dbHash, time.Now().UnixMilli())
	}
	return fi.Size(), int(q), true
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
	// 落盘时算文件头 hash（完整性校验用；hash 0=未知，仅 Put/StreamAndCache 落盘后写）
	hsh, _ := headHash(path)
	_, err := s.db.Exec(`INSERT INTO audio_cache (bvid,size,quality,hash,last_used,created_at)
		VALUES (?,?,?,?,?,?) ON CONFLICT(bvid) DO UPDATE SET size=?, quality=?, hash=?, last_used=?`,
		bvid, size, quality, hsh, now, now, size, quality, hsh, now)
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
// StreamAndCache 边转发给浏览器边 tee 落盘。contentLength 是上游声明的完整音频字节数
// （chunked 响应为 -1/0，见 http.Response.ContentLength）。完整性双判：
//   - 有 Content-Length：字节数 == contentLength（下载中断/seek 断连的截断文件直接丢弃，不污染缓存）；
//   - chunked/未知长度：io.Copy 无错误返回即读到完整 EOF（客户端断开会写 w 失败返回 err）才缓存；
//  否则一律丢弃（截断文件被 serve 后浏览器 seek 超范围→416→跳末尾→误切歌）。
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
	n, copyErr := io.Copy(io.MultiWriter(w, f), body)
	f.Close()
	complete := false
	switch {
	case contentLength > 0: // 上游声明完整长度：字节数必须一致（防截断）
		complete = n == contentLength
	default: // chunked / 未知长度：读到完整 EOF（copyErr==nil）才算完整下载
		complete = copyErr == nil
	}
	if complete {
		os.Rename(tmp, s.filePath(bvid))
		s.Put(bvid, quality, nil) // 已落盘，只写元数据 + 淘汰（Put 会读文件大小）
	} else {
		os.Remove(tmp)
	}
}

// ClearUnfav 只清不在 favs 表（未收藏）的歌。favTable 为收藏表名（跨域引用，避免循环依赖）；
// 拼接前白名单校验（review 前后不一 #2：字符串拼接进 SQL 虽两处一致，但风格危险）。
func (s *CacheService) ClearUnfav(favTable string) {
	favTable = conv.ValidateTableName(favTable)
	if favTable == "" {
		return
	}
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
