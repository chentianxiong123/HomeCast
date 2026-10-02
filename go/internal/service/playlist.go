package service

import (
	"database/sql"
	"fmt"
	"sync"

	"homecast/internal/bilibili"
)

// PlaylistItem 播放列表条目（对齐前端 MusicItem）
type PlaylistItem struct {
	BVID        string `json:"bvid"`
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	Cover       string `json:"cover"`
	Duration    string `json:"duration"`
	DurationSec int    `json:"duration_sec"`
}

// PlaylistResult 列表返回
type PlaylistResult struct {
	List []PlaylistItem `json:"list"`
}

// PlaylistService 播放列表（SQLite 持久化；首启迁移自旧 JSON，见 internal/store）
type PlaylistService struct {
	mu     sync.Mutex
	client *bilibili.Client
	db     *sql.DB
	list   []PlaylistItem
}

func NewPlaylistService(client *bilibili.Client, db *sql.DB) *PlaylistService {
	p := &PlaylistService{client: client, db: db}
	p.load()
	return p
}

func (p *PlaylistService) load() {
	rows, err := p.db.Query(`SELECT bvid,title,artist,cover,duration,duration_sec FROM playlist ORDER BY rowid DESC`)
	if err != nil {
		return
	}
	defer rows.Close()
	p.list = p.list[:0]
	for rows.Next() {
		var it PlaylistItem
		if rows.Scan(&it.BVID, &it.Title, &it.Artist, &it.Cover, &it.Duration, &it.DurationSec) == nil {
			p.list = append(p.list, it)
		}
	}
}

func (p *PlaylistService) save() {
	tx, err := p.db.Begin()
	if err != nil {
		return
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM playlist`); err != nil {
		return
	}
	for _, it := range p.list {
		if _, err := tx.Exec(`INSERT INTO playlist (bvid,title,artist,cover,duration,duration_sec) VALUES (?,?,?,?,?,?)`,
			it.BVID, it.Title, it.Artist, it.Cover, it.Duration, it.DurationSec); err != nil {
			return
		}
	}
	_ = tx.Commit()
}

// Get 当前列表
func (p *PlaylistService) Get() PlaylistResult {
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := make([]PlaylistItem, len(p.list))
	copy(cp, p.list)
	return PlaylistResult{List: cp}
}

// Add 加一首（已存在返回现有条目不重复添加）
func (p *PlaylistService) Add(bvid string) (*PlaylistItem, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.list {
		if p.list[i].BVID == bvid {
			it := p.list[i]
			return &it, nil
		}
	}
	info, err := p.client.GetVideoInfo(bvid)
	if err != nil {
		return nil, err
	}
	item := PlaylistItem{
		BVID:        info.BVID,
		Title:       info.Title,
		Artist:      info.Owner.Name,
		Cover:       info.Pic,
		Duration:    FormatDuration(info.Duration),
		DurationSec: info.Duration,
	}
	p.list = append(p.list, item)
	p.save()
	return &item, nil
}

// Remove 移除（bvid）
func (p *PlaylistService) Remove(bvid string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.list {
		if p.list[i].BVID == bvid {
			p.list = append(p.list[:i], p.list[i+1:]...)
			p.save()
			return true
		}
	}
	return false
}

// Clear 清空
func (p *PlaylistService) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.list = nil
	p.save()
}

// FormatDuration 秒 → "m:ss"
func FormatDuration(seconds int) string {
	m := seconds / 60
	s := seconds % 60
	return fmt.Sprintf("%d:%02d", m, s)
}