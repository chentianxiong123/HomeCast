package cache

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(CacheSchema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(SettingsSchema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func newTest(t *testing.T, limitMB int) (*CacheService, string) {
	t.Helper()
	dir := t.TempDir()
	audioDir := filepath.Join(dir, "audio")
	s := NewCacheService(testDB(t), audioDir, limitMB)
	return s, audioDir
}

// TestPutGet 写入→命中（文件存在 + 元数据）
func TestPutGet(t *testing.T) {
	s, dir := newTest(t, 500)
	if err := s.Put("BV1", 192, []byte("hello-world-audio-data")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "BV1.m4s")); err != nil {
		t.Fatalf("file missing: %v", err)
	}
	size, q, ok := s.Get("BV1")
	if !ok || size != int64(len("hello-world-audio-data")) || q != 192 {
		t.Fatalf("get mismatch: ok=%v size=%d q=%d", ok, size, q)
	}
}

// TestEvictLRU 超上限按 last_used 淘汰最旧
func TestEvictLRU(t *testing.T) {
	s, _ := newTest(t, 1) // 1MB 上限
	// 写 3 条各 ~0.4MB（>64KB 阈值），总 1.2MB 超限
	blob := make([]byte, 400*1024)
	for _, b := range []string{"old", "mid", "new"} {
		if err := s.Put(b, 192, blob); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond) // last_used 递增
	}
	// 上限 1MB，3×0.4MB=1.2MB → 应淘汰最旧的 "old"
	if _, _, ok := s.Get("old"); ok {
		t.Fatalf("old should be evicted")
	}
	if _, _, ok := s.Get("new"); !ok {
		t.Fatalf("new should remain")
	}
}

// TestClearUnfav 只清未收藏
func TestClearUnfav(t *testing.T) {
	s, dir := newTest(t, 500)
	if err := s.Put("fav1", 192, []byte("aaa")); err != nil {
		t.Fatal(err)
	}
	if err := s.Put("nofav1", 192, []byte("bbb")); err != nil {
		t.Fatal(err)
	}
	// 建 favs 表 + 插 fav1
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS favs (bvid TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO favs (bvid) VALUES ('fav1')`); err != nil {
		t.Fatal(err)
	}
	s.ClearUnfav("favs")
	if _, _, ok := s.Get("fav1"); !ok {
		t.Fatalf("fav1 should remain")
	}
	if _, _, ok := s.Get("nofav1"); ok {
		t.Fatalf("nofav1 should be cleared")
	}
	if _, err := os.Stat(filepath.Join(dir, "nofav1.m4s")); !os.IsNotExist(err) {
		t.Fatalf("nofav1 file should be removed")
	}
}

// TestClearAll 全清
func TestClearAll(t *testing.T) {
	s, dir := newTest(t, 500)
	if err := s.Put("BV1", 192, []byte("aaa")); err != nil {
		t.Fatal(err)
	}
	if err := s.Put("BV2", 192, []byte("bbb")); err != nil {
		t.Fatal(err)
	}
	s.ClearAll()
	if s.Count() != 0 {
		t.Fatalf("count should be 0, got %d", s.Count())
	}
	if _, err := os.Stat(filepath.Join(dir, "BV1.m4s")); !os.IsNotExist(err) {
		t.Fatalf("BV1 file should be removed")
	}
}

// TestSetLimit 设置上限立即淘汰
func TestSetLimit(t *testing.T) {
	s, _ := newTest(t, 0) // 初始 0=关
	blob := make([]byte, 400*1024)
	if err := s.Put("a", 192, blob); err != nil {
		t.Fatal(err)
	}
	if err := s.Put("b", 192, blob); err != nil {
		t.Fatal(err)
	}
	// 关缓存时 2 条都在
	if s.Count() != 2 {
		t.Fatalf("expected 2, got %d", s.Count())
	}
	// 设 1MB 上限 → 2×0.4MB=0.8MB 不超，仍 2 条
	s.SetLimitMB(1)
	if s.Count() != 2 {
		t.Fatalf("expected 2 after set 1MB, got %d", s.Count())
	}
	// 设 0.3MB → 立即淘汰到 ≤0.3MB（删到只剩 1 条）
	s.SetLimitMB(0)
	if s.Count() != 2 {
		t.Fatalf("0=关不清除, got %d", s.Count())
	}
}

// TestSize 已用大小
func TestSize(t *testing.T) {
	s, _ := newTest(t, 500)
	if err := s.Put("BV1", 192, make([]byte, 1024*1024)); err != nil { // 1MB
		t.Fatal(err)
	}
	if sz := s.Size(); sz < 0.99 || sz > 1.01 {
		t.Fatalf("size should be ~1MB, got %f", sz)
	}
}
