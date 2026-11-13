// Package widget 挂件域：桌面歌词挂件状态快照（desktop main 直读）。
// 全链路：入口 internal/api/widget.go；消费方 wails3/widget/（GTK/Win32 显示层）；表：无
package widget

import (
	"encoding/json"
	"log"
	"sync"
	"time"
)

// sseMsg SSE 广播载荷：name=事件名（toggle/lyric-state），data=JSON payload
// 用户理念：歌词状态走 SSE 推送（废轮询），挂件订阅即时消费；逐字由挂件本地平滑。
type SSEMsg struct {
	Name string
	Data string
}

// LyricSource 歌词行源（注入，解耦 widget 不直接依赖 lyric service）：
// 按 bvid 返回当前歌词行 [[sec,text,dur],...] + 对齐 offset（切歌才拉一次，低频）。
// 行格式对齐 lyric.LyricLine [3]any：[0]=sec(float64)、[1]=text(string)、[2]=dur(float64)。
type LyricSource interface {
	Lines(bvid string) (lines [][3]any, offset float64, ok bool)
}

// toFloat3 lyric [3]any 的 [0]/[2] 可能是 float64/int/json.Number，统一转 float64。
func toFloat3(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		if f, ok := toFloatAny(n); ok {
			return f
		}
	}
	return 0
}

// toFloatAny 兼容 float32/int64 等其他数值类型
func toFloatAny(v any) (float64, bool) {
	switch n := v.(type) {
	case float32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint64:
		return float64(n), true
	}
	return 0, false
}

// LyricState 桌面歌词挂件的播放状态（前端 WebView 上报，挂件同进程读取）
// ⏸ 控制用 SSE 推送（非轮询）：挂件按钮 → BroadcastToggle → 前端 EventSource 即时消费
// 歌词状态用 SSE 推送：Set → BroadcastLyricState → 挂件订阅即时更新（去 900ms 轮询）
// 多平台统一：位置→行 定位算法一份住后端（pickLine），SSE 带算好的 line，显示层纯渲染。
type WidgetState struct {
	mu          sync.Mutex
	BVID        string  `json:"bvid"`
	Title       string  `json:"title"`
	Artist      string  `json:"artist"`
	CurrentTime float64 `json:"current_time"`
	Duration    float64 `json:"duration"`
	Playing     bool    `json:"playing"`
	subs        map[chan SSEMsg]struct{} // SSE 订阅者（事件名+payload）

	// 日志去重（BroadcastLyricState 每次 Set 触发，250ms 一条；只打变化）
	lastLogBVID   string
	lastLogPlaying bool

	// 歌词行缓存（切歌才拉一次，低频；位置→行 定位唯一一份，住后端）
	lineSrc   LyricSource
	cacheBvid string
	cacheLines [][3]any
	cacheOff   float64
	cacheOK    bool
	pendingBvid string // 拉取在途的 bvid（防重复异步拉）
	lastFailAt time.Time // 最近一次拉取失败时间（去抖：失败后 5s 内不重拉）
}

// NewWidgetState 构造（lineSrc 可后续用 SetLineSrc 注入，避免循环依赖）
func NewWidgetState() *WidgetState {
	return &WidgetState{subs: make(map[chan SSEMsg]struct{})}
}

// SetLineSrc 注入歌词行源（server 建完 state 后注入，解耦）
func (w *WidgetState) SetLineSrc(src LyricSource) {
	w.mu.Lock()
	w.lineSrc = src
	w.mu.Unlock()
}

// InvalidateLines 清行缓存：offset/歌词源变更后强制下次 currentLine 重拉（用新 offset 算行）
func (w *WidgetState) InvalidateLines() {
	w.mu.Lock()
	w.cacheBvid = ""
	w.cacheLines = nil
	w.cacheOK = false
	w.pendingBvid = ""
	w.lastFailAt = time.Time{} // 清失败冷却：手动失效（选源/offset）后应立即可重拉
	w.mu.Unlock()
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

// BroadcastPrev 挂件 ◀ 按钮：请求上一首
func (w *WidgetState) BroadcastPrev() {
	w.broadcast(SSEMsg{Name: "prev", Data: "{}"})
}

// BroadcastNext 挂件 ▶ 按钮：请求下一首
func (w *WidgetState) BroadcastNext() {
	w.broadcast(SSEMsg{Name: "next", Data: "{}"})
}

// currentLine 位置→当前行 唯一逻辑（住后端）：ct+offset 对齐 t，找 sec<=t 最后一行；
// 无歌词/无行返回空（显示层退回标题）。
// 歌词行缓存：切歌（bvid 变）才异步预拉一次（低频、不阻塞 SSE），逐帧只用缓存定位。
func (w *WidgetState) currentLine2(bvid string, ct float64) (string, float64) {
	if bvid == "" {
		return "", 0
	}
	w.ensureLines(bvid)
	w.mu.Lock()
	lines, off, ok := w.cacheLines, w.cacheOff, w.cacheOK
	w.mu.Unlock()
	if !ok {
		return "", off
	}
	return pickLineAny(lines, ct+off), off
}

// ensureLines 确保 bvid 的行缓存：切歌才异步拉源（不阻塞调用方/SSE），拉到后广播刷新。
func (w *WidgetState) ensureLines(bvid string) {
	if w.lineSrc == nil {
		return
	}
	w.mu.Lock()
	need := w.cacheBvid != bvid || !w.cacheOK
	if need && !w.lastFailAt.IsZero() && time.Since(w.lastFailAt) < 5*time.Second {
		need = false // 拉取失败冷却中：不重试（去抖，防每次 currentLine 触发起 goroutine 刷屏）
	}
	if !need {
		w.mu.Unlock()
		return
	}
	// 标记正在拉（防重复），切歌后先置空行（显示层退标题，等拉到再刷）
	pending := w.pendingBvid == bvid
	if !pending {
		w.pendingBvid = bvid
		w.cacheBvid = bvid
		w.cacheLines = nil
		w.cacheOff = 0
		w.cacheOK = false
	}
	w.mu.Unlock()
	if pending {
		return // 已有一个拉取在途
	}
	// 异步拉：源可能联网（网易云搜歌词），不能阻塞 SSE；拉到后广播带真 line。
	// 拉取结束后必须清 pendingBvid（成功/失败都清）：否则首次拉取失败时
	// cacheOK=false + pendingBvid 残留 → 之后每次 ensureLines 都误判“在途”直接
	// return，该 bvid 的歌词行永远不会被拉到（真机实测：line 永远空）。
	b := bvid
	go func() {
		lines, off, ok := w.lineSrc.Lines(b)
		w.mu.Lock()
		if w.cacheBvid == b {
			w.cacheLines, w.cacheOff, w.cacheOK = lines, off, ok
		}
		if w.pendingBvid == b {
			w.pendingBvid = ""
		}
		if ok {
			w.lastFailAt = time.Time{} // 成功：清失败冷却
		} else {
			w.lastFailAt = time.Now() // 失败：冷却 5s，期间不重拉
		}
		w.mu.Unlock()
		if ok {
			w.BroadcastLyricState() // 拉到歌词，广播一次刷新（line 变真）
		}
	}()
}

// pickLineAny 位置→行 定位算法（唯一一份，住后端）：lines=[[sec,text,dur],...]，
// t=ct+offset，找 sec<=t 的最后一行。对齐 web lyric.js/subtitle.js（sec<=t 指哪打哪，
// KTV 进度由行 dur 字段，显示层处理；这里只定行）。
func pickLineAny(lines [][3]any, t float64) string {
	idx := -1
	for k, l := range lines {
		if toFloat3(l[0]) <= t {
			idx = k
		} else {
			break
		}
	}
	if idx < 0 {
		return ""
	}
	if s, ok := lines[idx][1].(string); ok {
		return s
	}
	return ""
}

// PickLine 公开包装（单测用）：位置→行 定位（唯一一份，住后端）
func PickLine(lines [][3]any, t float64) string { return pickLineAny(lines, t) }

// BroadcastLyricState 歌词状态变化（Set 时触发）：推送最新快照给挂件订阅者，
// 带后端算好的当前行 line（多平台统一：显示层只渲染 line，不本地定位）。
// 日志只在歌曲或播放态变化时打（Set 每 250ms 触发一次，全打是日志洪）。
func (w *WidgetState) BroadcastLyricState() {
	bvid, title, artist, ct, dur, playing := w.Get()
	line, offset := w.currentLine2(bvid, ct)
	if bvid != w.lastLogBVID || playing != w.lastLogPlaying {
		log.Printf("[widget] BroadcastLyricState bvid=%s ct=%.1f line=%q offset=%.1f", bvid, ct, line, offset)
		w.mu.Lock()
		w.lastLogBVID = bvid
		w.lastLogPlaying = playing
		w.mu.Unlock()
	}
	payload := map[string]any{
		"bvid": bvid, "title": title, "artist": artist,
		"current_time": ct, "duration": dur, "playing": playing,
		"line": line, "offset": offset,
	}
	b, _ := json.Marshal(payload)
	w.broadcast(SSEMsg{Name: "lyric-state", Data: string(b)})
}

// Set 更新（前端每 ~250ms 上报位置快照）→ 触发 SSE 推送（挂件即时更新，去轮询）
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


// currentLine 兼容包装（单测/外部只取 line）
func (w *WidgetState) currentLine(bvid string, ct float64) string {
	line, _ := w.currentLine2(bvid, ct)
	return line
}