package store

import (
	"path/filepath"
	"testing"
)

// TestOpenMigrate 打开 + 建表 + 写入读取（临时目录，不碰真实数据）
func TestOpenMigrate(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if err := Migrate(db, dir); err != nil { // 目录里无 JSON → 迁移应跳过不报错
		t.Fatalf("Migrate(无JSON): %v", err)
	}

	// favs 写入读取
	if _, err := db.Exec(`INSERT INTO favs (bvid,title,artist,cover,duration,duration_sec,play_count) VALUES ('BV1','t','a','c','3:00',180,0)`); err != nil {
		t.Fatalf("favs insert: %v", err)
	}
	var title string
	if err := db.QueryRow(`SELECT title FROM favs WHERE bvid='BV1'`).Scan(&title); err != nil {
		t.Fatalf("favs select: %v", err)
	}
	if title != "t" {
		t.Errorf("title = %q, want t", title)
	}

	// playlist 写入读取
	if _, err := db.Exec(`INSERT INTO playlist (bvid,title,artist,cover,duration,duration_sec) VALUES ('BV2','p','a','c','4:00',240)`); err != nil {
		t.Fatalf("playlist insert: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM playlist`).Scan(&n); err != nil { //nolint:execinquery
		t.Fatalf("playlist count: %v", err)
	}
	if n != 1 {
		t.Errorf("playlist count = %d, want 1", n)
	}

	// 幂等：再次 Migrate 不报错
	if err := Migrate(db, dir); err != nil {
		t.Fatalf("Migrate(二次): %v", err)
	}
}