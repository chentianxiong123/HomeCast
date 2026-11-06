// Package widget 挂件域：桌面歌词挂件状态快照（desktop main 直读）。
// 全链路：入口 internal/api/widget.go；消费方 wails3/widget/（GTK/Win32 显示层）；表：无
package widget

import (
	"sync"
)

// WidgetState 桌面歌词挂件的播放状态（前端 WebView 上报，挂件同进程读取）
// ⏸ 控制用 SSE 推送（非轮询）：挂件按钮 → BroadcastToggle → 前端 EventSource 即时消费
type WidgetState struct {
	mu          sync.Mutex
	BVID        string  `json:"bvid"`
	Title       string  `json:"title"`
	Artist      string  `json:"artist"`
	CurrentTime float64 `json:"current_time"`
	Duration    float64 `json:"duration"`
	Playing     bool    `json:"playing"`
	subs        map[chan struct{}]struct{} // SSE 订阅者（前端控制通道）
}

// NewWidgetState 构造
func NewWidgetState() *WidgetState {
	return &WidgetState{subs: make(map[chan struct{}]struct{})}
}

// Subscribe 注册 SSE 订阅者（返回 channel，BroadcastToggle 时收到信号）
func (w *WidgetState) Subscribe() chan struct{} {
	ch := make(chan struct{}, 1)
	w.mu.Lock()
	defer w.mu.Unlock()
	w.subs[ch] = struct{}{}
	return ch
}

// Unsubscribe 注销订阅者
func (w *WidgetState) Unsubscribe(ch chan struct{}) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.subs, ch)
}

// BroadcastToggle 挂件 ⏸ 按钮：向所有 SSE 订阅者即时推送（非阻塞）
func (w *WidgetState) BroadcastToggle() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for ch := range w.subs {
		select {
		case ch <- struct{}{}:
		default: // 通道满则丢弃（前端未及时消费，避免阻塞广播）
		}
	}
}

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
