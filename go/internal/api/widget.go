package api

import (
	"encoding/json"
	"net/http"

	"homecast/internal/service"
)

// WidgetHandler 桌面歌词挂件状态端点
type WidgetHandler struct {
	State *service.WidgetState
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