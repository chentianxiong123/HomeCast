package api

import (
	"encoding/json"
	"net/http"
	"time"

	"homecast/internal/service/widget"
)

// WidgetHandler 桌面歌词挂件状态端点
type WidgetHandler struct {
	State *widget.WidgetState
}

// Get GET /api/v1/widget/state
func (h *WidgetHandler) Get(w http.ResponseWriter, r *http.Request) {
	bvid, title, artist, ct, dur, playing := h.State.Get()
	ok(w, map[string]any{
		"bvid": bvid, "title": title, "artist": artist,
		"current_time": ct, "duration": dur, "playing": playing,
	})
}

// Set POST /api/v1/widget/state  body: {bvid, title, artist, current_time, duration, playing}
func (h *WidgetHandler) Set(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BVID        string  `json:"bvid"`
		Title       string  `json:"title"`
		Artist      string  `json:"artist"`
		CurrentTime float64 `json:"current_time"`
		Duration    float64 `json:"duration"`
		Playing     bool    `json:"playing"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errResp(w, http.StatusBadRequest, "invalid body")
		return
	}
	h.State.Set(body.BVID, body.Title, body.Artist, body.CurrentTime, body.Duration, body.Playing)
	ok(w, true)
}

// Toggle POST /api/v1/widget/toggle —— 挂件 ⏸ 按钮请求切换播放/暂停
// 经 SSE 即时推送给前端（非轮询），前端 EventSource 收到后调 audio.toggle()
func (h *WidgetHandler) Toggle(w http.ResponseWriter, r *http.Request) {
	h.State.BroadcastToggle()
	ok(w, true)
}

// Prev POST /api/v1/widget/prev —— 挂件 ◀ 按钮请求上一首
func (h *WidgetHandler) Prev(w http.ResponseWriter, r *http.Request) {
	h.State.BroadcastPrev()
	ok(w, true)
}

// Next POST /api/v1/widget/next —— 挂件 ▶ 按钮请求下一首
func (h *WidgetHandler) Next(w http.ResponseWriter, r *http.Request) {
	h.State.BroadcastNext()
	ok(w, true)
}

// Events GET /api/v1/widget/events —— SSE 下行控制流（挂件按钮 → 前端即时消费）
// 事件：event: toggle （前端收到即切换播放/暂停）；15s 心跳保活
func (h *WidgetHandler) Events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		errResp(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch := h.State.Subscribe()
	defer h.State.Unsubscribe(ch)
	// 初始握手，确认连接建立
	if _, err := w.Write([]byte(": connected\n\n")); err != nil {
		return
	}
	fl.Flush()
	// 心跳（防中间层超时断开）+ 推送（toggle / lyric-state 事件名+payload）
	hb := time.NewTicker(15 * time.Second)
	defer hb.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case m := <-ch:
			if _, err := w.Write([]byte("event: " + m.Name + "\ndata: " + m.Data + "\n\n")); err != nil {
				return
			}
			fl.Flush()
		case <-hb.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			fl.Flush()
		}
	}
}