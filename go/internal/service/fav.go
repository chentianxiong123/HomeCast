package service

import (
	"database/sql"
	"sync"
)

// FavSong 收藏条目（对齐前端 FavSong / MusicItem）
type FavSong struct {
	BVID        string `json:"bvid"`
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	Cover       string `json:"cover"`
	Duration    string `json:"duration"`
	DurationSec int    `json:"duration_sec"`
	PlayCount   int    `json:"play_count"`
}

// FavService 本地收藏（SQLite 持久化；首启迁移自旧 JSON，见 internal/store）
type FavService struct {
	mu sync.Mutex
	db *sql.DB
}

func NewFavService(db *sql.DB) *FavService {
	return &FavService{db: db}
}

func (f *FavService) load() []FavSong {
	rows, err := f.db.Query(`SELECT bvid,title,artist,cover,duration,duration_sec,play_count FROM favs ORDER BY rowid DESC`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var favs []FavSong
	for rows.Next() {
		var s FavSong
		if rows.Scan(&s.BVID, &s.Title, &s.Artist, &s.Cover, &s.Duration, &s.DurationSec, &s.PlayCount) == nil {
			favs = append(favs, s)
		}
	}
	return favs
}

// 保存：全表重写。favs 列表逻辑序是「新的在前」，SQLite 无 AUTOINCREMENT 时
// 空表 INSERT 复用最小 rowid（先插的 rowid 小）→ 必须倒序插入，rowid 递增才能
// 与 ORDER BY rowid DESC（新的在前）一致（sorted bug 曾出现：旧的在前）
func (f *FavService) save(favs []FavSong) error {
	tx, err := f.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM favs`); err != nil {
		return err
	}
	for i := len(favs) - 1; i >= 0; i-- {
		s := favs[i]
		if _, err := tx.Exec(`INSERT INTO favs (bvid,title,artist,cover,duration,duration_sec,play_count) VALUES (?,?,?,?,?,?,?)`,
			s.BVID, s.Title, s.Artist, s.Cover, s.Duration, s.DurationSec, s.PlayCount); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// List 收藏列表（新的在前）
func (f *FavService) List() []FavSong {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.load()
}

// Add 收藏：同 bvid 去重，插到最前
func (f *FavService) Add(entry FavSong) []FavSong {
	f.mu.Lock()
	defer f.mu.Unlock()
	favs := f.load()
	kept := favs[:0]
	for _, x := range favs {
		if x.BVID != entry.BVID {
			kept = append(kept, x)
		}
	}
	out := append([]FavSong{entry}, kept...)
	_ = f.save(out)
	return out
}

// Remove 取消收藏
func (f *FavService) Remove(bvid string) []FavSong {
	f.mu.Lock()
	defer f.mu.Unlock()
	favs := f.load()
	out := favs[:0]
	for _, x := range favs {
		if x.BVID != bvid {
			out = append(out, x)
		}
	}
	_ = f.save(out)
	return out
}

// Clear 清空全部收藏
func (f *FavService) Clear() []FavSong {
	f.mu.Lock()
	defer f.mu.Unlock()
	_ = f.save(nil)
	return nil
}