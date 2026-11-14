package api

import (
	"encoding/json"
	"net/http"
	"time"

	"homecast/internal/service/playback"
)

// StateHandler 统一播放状态端点（SSOT：后端唯一状态源）
type StateHandler struct {
	Pb *playback.PlaybackState
}

// Get GET /api/v1/state —— 当前快照
func (h *StateHandler) Get(w http.ResponseWriter, r *http.Request) {
	ok(w, h.Pb.Snapshot())
}

// Stream GET /api/v1/state/stream —— SSE 单流：所有组件订阅同一状态流
// 事件：state（全量快照，含队列/模式/来源）/ progress（进度更新）
func (h *StateHandler) Stream(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		errResp(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch := h.Pb.Subscribe()
	defer h.Pb.Unsubscribe(ch)
	// 初始：先推一次全量快照（新订阅者立即有状态）
	if b, err := json.Marshal(h.Pb.Snapshot()); err == nil {
		if _, err := w.Write([]byte("event: state\ndata: " + string(b) + "\n\n")); err != nil {
			return
		}
		fl.Flush()
	}
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

// Command POST /api/v1/state/command —— 统一命令入口
// body: {op: "play|pause|resume|toggle|seek|next|prev|mode|queue|switch_speaker|switch_web|remove|clear", ...}
func (h *StateHandler) Command(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Op     string  `json:"op"`
		BVID   string  `json:"bvid"`
		Title  string  `json:"title"`
		Artist string  `json:"artist"`
		Cover  string  `json:"cover"`
		Duration float64 `json:"duration"`
		Sec    float64 `json:"sec"`
		Mode   string  `json:"mode"`
		Did    string  `json:"did"`
		Queue  []playback.Song `json:"queue"`
		QIdx   int     `json:"qidx"`
		Volume int     `json:"volume"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errResp(w, http.StatusBadRequest, "invalid body")
		return
	}
	switch body.Op {
	case "play":
		h.Pb.Play(playback.Song{BVID: body.BVID, Title: body.Title, Artist: body.Artist, Cover: body.Cover, Duration: body.Duration})
	case "pause":
		h.Pb.Pause()
	case "resume":
		h.Pb.Resume()
	case "toggle":
		h.Pb.Toggle()
	case "seek":
		h.Pb.Seek(body.Sec)
	case "next":
		h.Pb.Next()
	case "prev":
		h.Pb.Prev()
	case "mode":
		h.Pb.SetMode(body.Mode)
	case "set_volume":
		h.Pb.SetVolume(body.Volume)
	case "queue":
		h.Pb.SetQueue(body.Queue, body.QIdx)
	case "switch_speaker":
		h.Pb.SwitchSpeaker(body.Did)
	case "stop_speaker":
		h.Pb.StopSpeaker(body.Did) // 设备页“停止”：当前音箱→停+回 web；非当前设备→no-op
	case "switch_web":
		h.Pb.SwitchWeb()
	case "remove":
		h.Pb.RemoveQueue(body.BVID)
	case "clear":
		h.Pb.ClearQueue()
	default:
		errResp(w, http.StatusBadRequest, "unknown op: "+body.Op)
		return
	}
	ok(w, true)
}

// Progress POST /api/v1/state/progress —— web 位置上报（高频，只更新进度，广播 progress）
// body: {position, duration, playing, bvid}
// bvid 校验：非空且 ≠ 当前播放歌 → 丢弃（多客户端：旧客户端/旧歌的进度不覆盖新歌）
func (h *StateHandler) Progress(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Position float64 `json:"position"`
		Duration float64 `json:"duration"`
		Playing  bool    `json:"playing"`
		BVID     string  `json:"bvid"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errResp(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.BVID != "" && body.BVID != h.Pb.Snapshot().BVID {
		ok(w, true) // 静默丢弃旧客户端/旧歌进度
		return
	}
	h.Pb.SetProgress(body.Position, body.Duration, body.Playing)
	// widget 不再这里写：核心广播（bcastProgress）→ server.go 唯一订阅者投影，
	// 单一喂入链（曾双写：此处理 + 700ms goroutine，切换瞬间双写歌词回跳）
	ok(w, true)
}
