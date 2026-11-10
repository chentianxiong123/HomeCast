package lyriccache

import (
	"database/sql"
	"path/filepath"
	"testing"

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
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestPutGet 写入→命中 + 刷新 last_used
func TestPutGet(t *testing.T) {
	s := NewLyricCache(testDB(t), 100)
	if err := s.Put("BV1", `{"lines":[["0","hello"]]}`); err != nil {
		t.Fatal(err)
	}
	e, ok := s.Get("BV1")
	if !ok || e.Data == "" {
		t.Fatal("miss after put")
	}
	if s.Count() != 1 {
		t.Fatalf("count=%d", s.Count())
	}
	// 未命中
	if _, ok := s.Get("BV2"); ok {
		t.Fatal("BV2 should miss")
	}
}

// TestLRUEvict 上限淘汰：超过 limit 删最旧
func TestLRUEvict(t *testing.T) {
	s := NewLyricCache(testDB(t), 2)
	s.Put("A", `{"l":"a"}`)
	s.Put("B", `{"l":"b"}`)
	s.Put("C", `{"l":"c"}`) // 超 2 → 淘汰最旧 A
	if s.Count() != 2 {
		t.Fatalf("count=%d want 2", s.Count())
	}
	if _, ok := s.Get("A"); ok {
		t.Fatal("A should be evicted")
	}
	if _, ok := s.Get("C"); !ok {
		t.Fatal("C should be present")
	}
}

// TestSetLimit 设置上限触发淘汰
func TestSetLimit(t *testing.T) {
	s := NewLyricCache(testDB(t), 100)
	s.Put("A", `{"l":"a"}`)
	s.Put("B", `{"l":"b"}`)
	s.SetLimit(1)
	if s.Count() != 1 {
		t.Fatalf("count=%d want 1 after setlimit", s.Count())
	}
}

// TestClearAll 清空
func TestClearAll(t *testing.T) {
	s := NewLyricCache(testDB(t), 100)
	s.Put("A", `{"l":"a"}`)
	s.ClearAll()
	if s.Count() != 0 {
		t.Fatal("should be empty")
	}
}

// TestClearUnfav 只清未收藏（favs 表）
func TestClearUnfav(t *testing.T) {
	s := NewLyricCache(testDB(t), 100)
	db := s.db
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS favs (bvid TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	db.Exec(`INSERT INTO favs (bvid) VALUES ('A')`)
	s.Put("A", `{"l":"a"}`)
	s.Put("B", `{"l":"b"}`)
	s.ClearUnfav("favs")
	if _, ok := s.Get("A"); !ok {
		t.Fatal("A faved should stay")
	}
	if _, ok := s.Get("B"); ok {
		t.Fatal("B unfav should be cleared")
	}
}

// TestRemove 删单条（选源/偏移变更）
func TestRemove(t *testing.T) {
	s := NewLyricCache(testDB(t), 100)
	s.Put("A", `{"l":"a"}`)
	s.Remove("A")
	if _, ok := s.Get("A"); ok {
		t.Fatal("A should be removed")
	}
}

// TestPersistLimit 上限持久化
func TestPersistLimit(t *testing.T) {
	db := testDB(t)
	s := NewLyricCache(db, 100)
	s.SetLimit(200)
	s2 := NewLyricCache(db, -1) // 从持久化读
	if s2.Limit() != 200 {
		t.Fatalf("limit=%d want 200", s2.Limit())
	}
}
