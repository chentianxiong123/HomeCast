package service

import (
	"path/filepath"
	"testing"

	"homecast/internal/store"
)

func newTestPlaylist(t *testing.T) *PlaylistService {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(db, t.TempDir()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewPlaylistService(nil, db) // client=nil：不联网路径（已存在/移除/清空）不受影响
}

// TestPlaylistAddExistingDedup 已存在 bvid 添加：命中缓存直接返回，不调 client（nil client 不 panic 即通过）
func TestPlaylistAddExistingDedup(t *testing.T) {
	p := newTestPlaylist(t)
	db := p.db
	// 直接种一条（避免 Add 未存在路径需要 client）
	if _, err := db.Exec(`INSERT INTO playlist (bvid,title,artist,cover,duration,duration_sec) VALUES ('BV1','一','a','c','3:00',180)`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	p.load()

	it, err := p.Add("BV1")
	if err != nil {
		t.Fatalf("Add(已存在): %v", err)
	}
	if it.BVID != "BV1" || it.Title != "一" {
		t.Errorf("Add(已存在) 返回 %+v, want BV1/一", it)
	}
	if got := p.Get().List; len(got) != 1 {
		t.Errorf("重复添加后 len=%d, want 1（不重复）", len(got))
	}
}

// TestPlaylistRemove 移除存在/不存在
func TestPlaylistRemove(t *testing.T) {
	p := newTestPlaylist(t)
	if _, err := p.db.Exec(`INSERT INTO playlist (bvid,title,artist,cover,duration,duration_sec) VALUES ('BV1','一','a','c','3:00',180),('BV2','二','b','c','4:00',240)`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	p.load()

	if !p.Remove("BV1") {
		t.Fatal("Remove(存在) 应返回 true")
	}
	if len(p.Get().List) != 1 {
		t.Errorf("Remove 后 len=%d, want 1", len(p.Get().List))
	}
	if p.Remove("BV999") {
		t.Error("Remove(不存在) 应返回 false")
	}

	p.Clear()
	if got := p.Get().List; len(got) != 0 {
		t.Errorf("Clear 后 len=%d, want 0", len(got))
	}
}