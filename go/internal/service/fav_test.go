package service

import (
	"path/filepath"
	"testing"

	"homecast/internal/store"
)

func newTestFav(t *testing.T) *FavService {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(db, t.TempDir()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewFavService(db)
}

func song(bvid, title string) FavSong {
	return FavSong{BVID: bvid, Title: title, Artist: "a", Cover: "c", Duration: "3:00", DurationSec: 180}
}

// TestFavAddListRemoveClear 完整收藏生命周期：新增 → 列表 → 取消 → 清空
func TestFavAddListRemoveClear(t *testing.T) {
	f := newTestFav(t)

	if got := f.Add(song("BV1", "一")); len(got) != 1 {
		t.Fatalf("Add 后 len=%d, want 1", len(got))
	}
	f.Add(song("BV2", "二"))
	list := f.List()
	if len(list) != 2 {
		t.Fatalf("List len=%d, want 2", len(list))
	}
	if list[0].BVID != "BV2" { // 新的在前
		t.Errorf("List[0]=%s, want BV2（新的在前）", list[0].BVID)
	}

	f.Remove("BV1")
	if got := f.List(); len(got) != 1 || got[0].BVID != "BV2" {
		t.Errorf("Remove 后 = %+v, want 只剩 BV2", got)
	}
	// 移除不存在的 bvid：不炸
	f.Remove("BV999")
	if got := f.List(); len(got) != 1 {
		t.Errorf("移除不存在后 len=%d, want 1", len(got))
	}

	f.Clear()
	if got := f.List(); len(got) != 0 {
		t.Errorf("Clear 后 len=%d, want 0", len(got))
	}
}

// TestFavAddDedup 同 bvid 重收藏：不重复，且用新数据
func TestFavAddDedup(t *testing.T) {
	f := newTestFav(t)
	f.Add(song("BV1", "旧名"))
	f.Add(FavSong{BVID: "BV1", Title: "新名", Artist: "b", Cover: "c", Duration: "1:00", DurationSec: 60})

	list := f.List()
	if len(list) != 1 {
		t.Fatalf("重复 Add 后 len=%d, want 1", len(list))
	}
	if list[0].Title != "新名" {
		t.Errorf("Title=%q, want 新名（重复收藏更新数据）", list[0].Title)
	}
}

// TestFavPersist 数据落盘：重建 service（同一 db 文件）数据仍在
func TestFavPersist(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, dir); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	f1 := NewFavService(db)
	f1.Add(song("BV9", "九"))
	db.Close()

	db2, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("reopen db: %v", err)
	}
	defer db2.Close()
	f2 := NewFavService(db2)
	if got := f2.List(); len(got) != 1 || got[0].BVID != "BV9" {
		t.Errorf("重建后 = %+v, want BV9 仍在", got)
	}
}