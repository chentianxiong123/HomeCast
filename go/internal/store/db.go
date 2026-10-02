// Package store 本地持久层：SQLite 单文件（modernc.org/sqlite，无 CGO，安卓可交叉编译）
// 只负责打开 / 建表 / 首启迁移（JSON → SQLite）；业务 CRUD 写在各自功能切片内
package store

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Open 打开（不存在则创建）SQLite 文件
func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	return db, nil
}

// Migrate 建表 + 首启迁移：历史 JSON 数据导入 SQLite（JSON 文件保留只读备份）
func Migrate(db *sql.DB, dataDir string) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS favs (
			bvid TEXT PRIMARY KEY, title TEXT, artist TEXT, cover TEXT,
			duration TEXT, duration_sec INTEGER, play_count INTEGER, created_at INTEGER DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS playlist (
			bvid TEXT PRIMARY KEY, title TEXT, artist TEXT, cover TEXT,
			duration TEXT, duration_sec INTEGER, created_at INTEGER DEFAULT 0)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	if err := migrateJSON(db, "favs", filepath.Join(dataDir, "favorites.json")); err != nil {
		return err
	}
	return migrateJSON(db, "playlist", filepath.Join(dataDir, "playlist.json"))
}

// jsonRow 历史 JSON 条目（对齐 FavSong / PlaylistItem 字段）
type jsonRow struct {
	BVID        string `json:"bvid"`
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	Cover       string `json:"cover"`
	Duration    string `json:"duration"`
	DurationSec int    `json:"duration_sec"`
	PlayCount   int    `json:"play_count"`
}

// migrateJSON JSON → 表（仅当表空且 JSON 存在；JSON 损坏或缺读按空处理不崩）
func migrateJSON(db *sql.DB, table, jsonPath string) error {
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil // 已有数据，不重复迁移
	}
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil // 无历史 JSON，正常
	}
	var rows []jsonRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil
	}
	for _, r := range rows {
		if r.BVID == "" {
			continue
		}
		var err error
		if table == "favs" {
			_, err = db.Exec(`INSERT OR IGNORE INTO favs (bvid,title,artist,cover,duration,duration_sec,play_count) VALUES (?,?,?,?,?,?,?)`,
				r.BVID, r.Title, r.Artist, r.Cover, r.Duration, r.DurationSec, r.PlayCount)
		} else {
			_, err = db.Exec(`INSERT OR IGNORE INTO playlist (bvid,title,artist,cover,duration,duration_sec) VALUES (?,?,?,?,?,?)`,
				r.BVID, r.Title, r.Artist, r.Cover, r.Duration, r.DurationSec)
		}
		if err != nil {
			return err
		}
	}
	return nil
}