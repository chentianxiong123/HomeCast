// Package lyriccache 歌词行缓存域：后端持有（SQLite TEXT 存歌词行 JSON），与音频缓存同逻辑。
//
// 全链路：入口 api/lyric.go Get（透明缓存：命中直接返回，未命中拉取后 Put）；表：lyric_cache。
// 设计对齐音频缓存（cache 包）：
//   - LRU 淘汰（last_used 最旧先删）
//   - 上限持久化（settings kv，条目数——歌词是文本小数据，条目数比 MB 合理）
//   - 双清除（ClearAll 全清 / ClearUnfav 只清未收藏）
//   - 歌词行存 SQLite TEXT（不像音频大文件落盘——歌词文本小，塞 DB 合理；管理逻辑与音频一致）
//
// 前端 lyric.js 不再用 localStorage 缓存歌词行；后端透明缓存（Get 命中秒显/离线可看）。
package lyriccache

import (
	"database/sql"
	"encoding/json"
	"log"
	"sync"
	"time"
)

// CacheTable lyric_cache 表名
const CacheTable = "lyric_cache"

// CacheSchema 表定义（建表执行在 store.Migrate 统一跑）
const CacheSchema = `CREATE TABLE IF NOT EXISTS lyric_cache (
	bvid TEXT PRIMARY KEY, data TEXT, size INTEGER DEFAULT 0,
	last_used INTEGER DEFAULT 0, created_at INTEGER DEFAULT 0)`

// DefaultLimit 默认缓存条目上限（首）
const DefaultLimit = 500

// limitKey settings kv 键（条目数上限）
const limitKey = "lyric_cache_limit"

// Entry 缓存条目（data 为 LyricResult JSON 序列化）
type Entry struct {
	Data string
	Size int64
}

// LyricCache 歌词行缓存（SQLite，LRU + 上限 + 双清除）
type LyricCache struct {
	mu      sync.Mutex
	db      *sql.DB
	limit   int
}

// NewLyricCache 构造（limit<0 用默认或持久化值）
func NewLyricCache(db *sql.DB, limit int) *LyricCache {
	s := &LyricCache{db: db, limit: DefaultLimit}
	if limit >= 0 {
		s.limit = limit
	} else if v, err := s.loadLimit(); err == nil {
		s.limit = v
	}
	return s
}

func (s *LyricCache) loadLimit() (int, error) {
	var v string
	if err := s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, limitKey).Scan(&v); err != nil {
		return 0, err
	}
	n := 0
	for _, c := range v {
		if c < '0' || c > '9' {
			return DefaultLimit, nil
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

// Limit 当前上限（条目数）
func (s *LyricCache) Limit() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.limit
}

// SetLimit 设置上限并持久化（0=关缓存）
func (s *LyricCache) SetLimit(v int) {
	if v < 0 {
		v = 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.limit = v
	s.db.Exec(`INSERT INTO settings (key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=?`,
		limitKey, itoa(v), itoa(v))
	s.evictLocked() // 降低上限可能触发立即淘汰
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

// Get 命中返回 Entry + 刷新 last_used（LRU 触碰）；未命中 ok=false
func (s *LyricCache) Get(bvid string) (Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var e Entry
	var lastUsed int64
	err := s.db.QueryRow(`SELECT data, size, last_used FROM lyric_cache WHERE bvid=?`, bvid).Scan(&e.Data, &e.Size, &lastUsed)
	if err != nil {
		return Entry{}, false
	}
	if e.Data == "" {
		return Entry{}, false
	}
	now := time.Now().UnixMilli()
	s.db.Exec(`UPDATE lyric_cache SET last_used=? WHERE bvid=?`, now, bvid)
	return e, true
}

// Put 写入缓存（data 为 JSON 字符串）+ LRU 淘汰
func (s *LyricCache) Put(bvid string, data string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UnixMilli()
	size := int64(len(data))
	_, err := s.db.Exec(`INSERT INTO lyric_cache (bvid,data,size,last_used,created_at)
		VALUES (?,?,?,?,?) ON CONFLICT(bvid) DO UPDATE SET data=?, size=?, last_used=?`,
		bvid, data, size, now, now, data, size, now)
	if err != nil {
		return err
	}
	s.evictLocked()
	return nil
}

// evictLocked LRU 淘汰：条目数超上限删最旧 last_used（需持锁）
func (s *LyricCache) evictLocked() {
	if s.limit <= 0 {
		return
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM lyric_cache`).Scan(&n); err != nil {
		return
	}
	if n <= s.limit {
		return
	}
	rows, err := s.db.Query(`SELECT bvid, last_used FROM lyric_cache ORDER BY last_used ASC`)
	if err != nil {
		return
	}
	type ent struct {
		bvid string
		lu   int64
	}
	var ents []ent
	for rows.Next() {
		var b string
		var lu int64
		if rows.Scan(&b, &lu) == nil {
			ents = append(ents, ent{b, lu})
		}
	}
	rows.Close()
	// 删最旧直到达标
	for _, e := range ents {
		if n <= s.limit {
			break
		}
		s.db.Exec(`DELETE FROM lyric_cache WHERE bvid=?`, e.bvid)
		n--
	}
}

// Count 条目数
func (s *LyricCache) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	var n int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM lyric_cache`).Scan(&n)
	return n
}

// SizeBytes 总字节（歌词文本小，MB 展示意义不大，但保留统一 Stats）
func (s *LyricCache) SizeBytes() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	var total int64
	rows, err := s.db.Query(`SELECT size FROM lyric_cache`)
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
	return total
}

// Remove 删单条（选源/偏移变更时清该 bvid 缓存，避免旧数据误命中）
func (s *LyricCache) Remove(bvid string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Exec(`DELETE FROM lyric_cache WHERE bvid=?`, bvid)
}

// ClearAll 清空全部歌词缓存
func (s *LyricCache) ClearAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Exec(`DELETE FROM lyric_cache`)
}

// ClearUnfav 只清不在 favs 表（未收藏）的歌
func (s *LyricCache) ClearUnfav(favTable string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db.Exec(`DELETE FROM lyric_cache WHERE bvid NOT IN (SELECT bvid FROM ` + favTable + `)`)
}

// Marshal 把任意缓存数据序列化为 JSON 字符串（供 Put）
func Marshal(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		log.Printf("[lyriccache] marshal: %v", err)
		return ""
	}
	return string(b)
}

// Unmarshal JSON → 目标结构（供 Get 命中后解码）
func Unmarshal(data string, v any) error {
	return json.Unmarshal([]byte(data), v)
}
