package service

import (
	"encoding/json"
	"os"
	"path/filepath"
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

// FavService 本地 JSON 收藏（对齐桌面 music.py：去重置顶，新的在前）
// 注意：json.Marshal 对非法 UTF-8 自动替换 U+FFFD 不抛错，写盘永不崩
type FavService struct {
	mu   sync.Mutex
	path string
}

func NewFavService(path string) *FavService {
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".config", "homecast", "favorites.json")
	}
	return &FavService{path: path}
}

func (f *FavService) load() []FavSong {
	data, err := os.ReadFile(f.path)
	if err != nil {
		return nil
	}
	var favs []FavSong
	if err := json.Unmarshal(data, &favs); err != nil {
		return nil // 文件损坏当空处理，不崩
	}
	return favs
}

func (f *FavService) save(favs []FavSong) error {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(favs)
	if err != nil {
		return err
	}
	return os.WriteFile(f.path, data, 0o644)
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