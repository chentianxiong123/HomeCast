package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// PlaylistService 播放列表（本地 JSON 持久化，对齐 Python playlist_service）
type PlaylistService struct {
	mu     sync.Mutex
	client *bilibili.Client
	path   string
	list   []PlaylistItem
}

func NewPlaylistService(client *bilibili.Client, path string) *PlaylistService {
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".config", "homecast", "playlist.json")
	}
	p := &PlaylistService{client: client, path: path}
	p.load()
	return p
}

func (p *PlaylistService) load() {
	data, err := os.ReadFile(p.path)
	if err != nil {
		return
	}
	var list []PlaylistItem
	if json.Unmarshal(data, &list) == nil {
		p.list = list
	}
}

func (p *PlaylistService) save() {
	if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err != nil {
		return
	}
	if data, err := json.Marshal(p.list); err == nil {
		_ = os.WriteFile(p.path, data, 0o644)
	}
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