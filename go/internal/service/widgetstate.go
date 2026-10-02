package service

import (
	"sync"
)

// WidgetState 桌面歌词挂件的播放状态（前端 WebView 上报，挂件同进程读取）
type WidgetState struct {
	mu          sync.Mutex
	BVID        string  `json:"bvid"`
	Title       string  `json:"title"`
	Artist      string  `json:"artist"`
	CurrentTime float64 `json:"current_time"`
	Duration    float64 `json:"duration"`
	Playing     bool    `json:"playing"`
}

// NewWidgetState 构造
func NewWidgetState() *WidgetState { return &WidgetState{} }

// Set 更新（前端每 ~1s 上报校准）
func (w *WidgetState) Set(bvid, title, artist string, currentTime, duration float64, playing bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.BVID = bvid
	w.Title = title
	w.Artist = artist
	w.CurrentTime = currentTime
	w.Duration = duration
	w.Playing = playing
}

// Get 快照
func (w *WidgetState) Get() (bvid, title, artist string, currentTime, duration float64, playing bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.BVID, w.Title, w.Artist, w.CurrentTime, w.Duration, w.Playing
}