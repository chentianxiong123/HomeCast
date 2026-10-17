package lyric

import (
	"database/sql"
	"path/filepath"
	"testing"

	"homecast/internal/store"
)

func newTestLyricStore(t *testing.T) *LyricStore {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, []string{LyricSchema}, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewLyricStore(db)
}

// 基础 CRUD：选候选 → 读回；换候选 → 覆盖；删候选(sid<=0) → 清记录
func TestLyricStoreCRUD(t *testing.T) {
	s := newTestLyricStore(t)
	const bvid = "BV1abc"
	if got := s.SelectedSID(bvid); got != 0 {
		t.Fatalf("初始 SelectedSID = %d, want 0", got)
	}
	s.SetSelection(bvid, LyricSel{BVID: bvid, SID: 1001, Name: "版本A", Artist: "歌手A", Duration: 210})
	if got := s.SelectedSID(bvid); got != 1001 {
		t.Fatalf("选中后 SelectedSID = %d, want 1001", got)
	}
	sel, ok := s.Get(bvid)
	if !ok || sel.Name != "版本A" || sel.Artist != "歌手A" || sel.Duration != 210 {
		t.Fatalf("Get = %+v ok=%v, want 版本A/歌手A/210", sel, ok)
	}
	// 换候选：覆盖，且已调偏移保留
	s.SetOffset(bvid, 1.5)
	s.SetSelection(bvid, LyricSel{BVID: bvid, SID: 1002, Name: "版本B"})
	sel, _ = s.Get(bvid)
	if sel.SID != 1002 || sel.Name != "版本B" || sel.Offset != 1.5 {
		t.Fatalf("换候选后 = %+v, want SID=1002 Name=版本B Offset=1.5", sel)
	}
	// 删除
	s.SetSelection(bvid, LyricSel{BVID: bvid, SID: 0})
	if got := s.SelectedSID(bvid); got != 0 {
		t.Fatalf("删除后 SelectedSID = %d, want 0", got)
	}
}

// 对齐偏移：无选择时也能单独存偏移（只记 bvid → offset）
func TestLyricStoreSetOffsetOnly(t *testing.T) {
	s := newTestLyricStore(t)
	s.SetOffset("BV1only", -0.5)
	sel, ok := s.Get("BV1only")
	if !ok || sel.SID != 0 || sel.Offset != -0.5 {
		t.Fatalf("仅偏移 = %+v ok=%v, want SID=0 Offset=-0.5", sel, ok)
	}
}

// 重启持久化：同一 db 文件换新 store 实例，数据不丢（这是“用户数据”而不是缓存的核心断言）
func TestLyricStorePersistAcrossRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "persist.db")
	db1, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := store.Migrate(db1, []string{LyricSchema}, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	s1 := NewLyricStore(db1)
	s1.SetSelection("BV1persist", LyricSel{BVID: "BV1persist", SID: 777, Name: "持久化版本", Artist: "歌手"})
	s1.SetOffset("BV1persist", 2.0)
	s1.SetOffset("BV1o", -1.5) // 只有偏移、没选候选
	if err := db1.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	db2, err := store.Open(dbPath) // 模拟服务重启
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db2.Close()
	s2 := NewLyricStore(db2)
	sel, ok := s2.Get("BV1persist")
	if !ok || sel.SID != 777 || sel.Name != "持久化版本" || sel.Offset != 2.0 {
		t.Fatalf("重启后 = %+v ok=%v, want SID=777 持久化版本 Offset=2.0", sel, ok)
	}
	solo, ok := s2.Get("BV1o")
	if !ok || solo.Offset != -1.5 {
		t.Fatalf("重启后仅偏移 = %+v, want Offset=-1.5", solo)
	}
}

// 内存模式（db==nil）：语义一致，测试/无库环境不崩
func TestLyricStoreMemoryMode(t *testing.T) {
	s := NewLyricStore(nil)
	s.SetSelection("BVmem", LyricSel{BVID: "BVmem", SID: 5, Name: "内存版"})
	s.SetOffset("BVmem", 0.5)
	if got := s.SelectedSID("BVmem"); got != 5 {
		t.Fatalf("内存模式 SelectedSID = %d, want 5", got)
	}
	sel, ok := s.Get("BVmem")
	if !ok || sel.Offset != 0.5 {
		t.Fatalf("内存模式 Get = %+v, want Offset=0.5", sel)
	}
	s.SetSelection("BVmem", LyricSel{BVID: "BVmem", SID: 0})
	if got := s.SelectedSID("BVmem"); got != 0 {
		t.Fatalf("内存模式删除后 = %d, want 0", got)
	}
}

var _ = sql.ErrNoRows