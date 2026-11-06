// Package widget 挂件域：桌面歌词挂件状态快照（desktop main 直读）。
// 全链路：入口 internal/api/widget.go；消费方 wails3/widget/（GTK/Win32 显示层）；表：无
package widget

import (
	"encoding/json"
	"sync"
)

// sseMsg SSE 广播载荷：name=事件名（toggle/lyric-state），data=JSON payload
// 用户理念：歌词状态走 SSE 推送（废轮询），挂件订阅即时消费；逐字由挂件本地平滑。
type SSEMsg struct {
	Name string
	Data string
}

// WidgetState 桌面歌词挂件的播放状态（前端 WebView 上报，挂件同进程读取）
// ⏸ 控制用 SSE 推送（非轮询）：挂件按钮 → BroadcastToggle → 前端 EventSource 即时消费
// 歌词状态用 SSE 推送：Set → BroadcastLyricState → 挂件订阅即时更新（去 900ms 轮询）
type WidgetState struct {
	mu          sync.Mutex
	BVID        string  `json:"bvid"`
	Title       string  `json:"title"`
	Artist      string  `json:"artist"`
	CurrentTime float64 `json:"current_time"`
	Duration    float64 `json:"duration"`
	Playing     bool    `json:"playing"`
	subs        map[chan SSEMsg]struct{} // SSE 订阅者（事件名+payload）
}

// NewWidgetState 构造
func NewWidgetState() *WidgetState {
	return &WidgetState{subs: make(map[chan SSEMsg]struct{})}
}

// Subscribe 注册 SSE 订阅者（返回 channel，广播时收到事件名+payload）
func (w *WidgetState) Subscribe() chan SSEMsg {
	ch := make(chan SSEMsg, 8)
	w.mu.Lock()
	defer w.mu.Unlock()
	w.subs[ch] = struct{}{}
	return ch
}

// Unsubscribe 注销订阅者
func (w *WidgetState) Unsubscribe(ch chan SSEMsg) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.subs, ch)
}

// broadcast 向所有 SSE 订阅者即时推送（非阻塞）
func (w *WidgetState) broadcast(m SSEMsg) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for ch := range w.subs {
		select {
		case ch <- m:
		default: // 通道满则丢弃（前端未及时消费，避免阻塞广播）
		}
	}
}

// BroadcastToggle 挂件 ⏸ 按钮：向所有 SSE 订阅者即时推送（非阻塞）
func (w *WidgetState) BroadcastToggle() {
	w.broadcast(SSEMsg{Name: "toggle", Data: "{}"})
}

// BroadcastLyricState 歌词状态变化（Set 时触发）：推送最新快照给挂件订阅者
func (w *WidgetState) BroadcastLyricState() {
	bvid, title, artist, ct, dur, playing := w.Get()
	payload := map[string]any{
		"bvid": bvid, "title": title, "artist": artist,
		"current_time": ct, "duration": dur, "playing": playing,
	}
	b, _ := json.Marshal(payload)
	w.broadcast(SSEMsg{Name: "lyric-state", Data: string(b)})
}

// Set 更新（前端每 ~1s 上报校准）→ 触发 SSE 推送（挂件即时更新，去轮询）
func (w *WidgetState) Set(bvid, title, artist string, currentTime, duration float64, playing bool) {
	w.mu.Lock()
	w.BVID = bvid
	w.Title = title
	w.Artist = artist
	w.CurrentTime = currentTime
	w.Duration = duration
	w.Playing = playing
	w.mu.Unlock()
	w.BroadcastLyricState()
}

// Get 快照
func (w *WidgetState) Get() (bvid, title, artist string, currentTime, duration float64, playing bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.BVID, w.Title, w.Artist, w.CurrentTime, w.Duration, w.Playing
}
