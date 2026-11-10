// Package playback 播放域统一状态源（SSOT）：业务状态（当前歌/播放/进度/队列/模式/来源）
// 物理唯一住 Go 后端。前端只发命令（POST /api/v1/state/command）+ 订阅一条 SSE
// （/api/v1/state/stream）渲染。消除 web/音箱/后端 3 处状态硬拼接。
//
// 全链路：入口 internal/api/state.go；消费方前端（面板/歌词/字幕/挂件）订阅同一流；表：state（持久化，阶段4）
package playback

import (
	"encoding/json"
	"log"
	"sync"
)

// Song 歌曲（对齐前端 song / fav.FavSong 字段）
type Song struct {
	BVID     string `json:"bvid"`
	Title    string `json:"title"`
	FullTitle string `json:"fullTitle,omitempty"`
	Artist   string `json:"artist"`
	Cover    string `json:"cover"`
	Duration float64 `json:"duration,omitempty"`
}

// SSE 广播载荷（对齐 widget.SSEMsg）
type SSEMsg struct {
	Name string // 事件名：state（全量快照）/ progress（进度更新）
	Data string
}

// State 唯一状态源快照（前端渲染依据）
type State struct {
	Source   string `json:"source"`   // web | speaker
	BVID     string `json:"bvid"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Cover    string `json:"cover"`
	Playing  bool   `json:"playing"`
	Position float64 `json:"position"`
	Duration float64 `json:"duration"`
	PlayMode string `json:"play_mode"` // order/loop/single/random
	Queue    []Song `json:"queue"`
	QIdx     int    `json:"qidx"`
	SpkDID   string `json:"spk_did,omitempty"` // source=speaker 时目标设备 did
}

// PlaybackState 统一状态管理器（线程安全）
type PlaybackState struct {
	mu   sync.Mutex
	cur  State
	subs map[chan SSEMsg]struct{}
}

// NewPlaybackState 构造
func NewPlaybackState() *PlaybackState {
	return &PlaybackState{
		cur: State{Source: "web", PlayMode: "order", QIdx: -1, Queue: []Song{}},
		subs: make(map[chan SSEMsg]struct{}),
	}
}

// Subscribe 注册 SSE 订阅者
func (p *PlaybackState) Subscribe() chan SSEMsg {
	ch := make(chan SSEMsg, 16)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.subs[ch] = struct{}{}
	return ch
}

// Unsubscribe 注销
func (p *PlaybackState) Unsubscribe(ch chan SSEMsg) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.subs, ch)
}

// broadcast 非阻塞推送给所有订阅者（通道满丢弃，防阻塞）
func (p *PlaybackState) broadcast(m SSEMsg) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for ch := range p.subs {
		select {
		case ch <- m:
		default:
		}
	}
}

// Snapshot 当前快照（深拷贝，调用方可安全读）
func (p *PlaybackState) Snapshot() State {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.cur
	s.Queue = append([]Song(nil), p.cur.Queue...)
	return s
}

// bcastSnapshot 广播全量快照（state 事件）
func (p *PlaybackState) bcastSnapshot() {
	s := p.Snapshot()
	b, _ := json.Marshal(s)
	p.broadcast(SSEMsg{Name: "state", Data: string(b)})
}

// bcastProgress 广播进度（progress 事件，高频但轻量，只带 position/duration/playing）
func (p *PlaybackState) bcastProgress() {
	p.mu.Lock()
	s := p.cur
	p.mu.Unlock()
	b, _ := json.Marshal(map[string]any{
		"source": s.Source, "bvid": s.BVID, "playing": s.Playing,
		"position": s.Position, "duration": s.Duration, "spk_did": s.SpkDID,
	})
	p.broadcast(SSEMsg{Name: "progress", Data: string(b)})
}

// ---- 命令（前端发命令 → 后端改状态 → 广播） ----

// Play 播放某歌：入队（去重）+ 定位 + 播（source 保持；web 由前端执行 audio.play，音箱由后端推）
func (p *PlaybackState) Play(s Song) {
	p.mu.Lock()
	// 去重后入队尾
	for i, q := range p.cur.Queue {
		if q.BVID == s.BVID {
			p.cur.Queue = append(p.cur.Queue[:i], p.cur.Queue[i+1:]...)
			break
		}
	}
	p.cur.Queue = append(p.cur.Queue, s)
	p.cur.QIdx = len(p.cur.Queue) - 1
	p.cur.BVID = s.BVID
	p.cur.Title = s.Title
	p.cur.Artist = s.Artist
	p.cur.Cover = s.Cover
	p.cur.Duration = s.Duration
	p.cur.Position = 0
	p.cur.Playing = true
	p.mu.Unlock()
	log.Printf("[playback] Play bvid=%s source=%s", s.BVID, p.Snapshot().Source)
	p.bcastSnapshot()
}

// Pause 暂停
func (p *PlaybackState) Pause() {
	p.mu.Lock()
	p.cur.Playing = false
	p.mu.Unlock()
	p.bcastProgress()
}

// Resume 恢复
func (p *PlaybackState) Resume() {
	p.mu.Lock()
	p.cur.Playing = true
	p.mu.Unlock()
	p.bcastProgress()
}

// Toggle 播放/暂停切换
func (p *PlaybackState) Toggle() {
	p.mu.Lock()
	p.cur.Playing = !p.cur.Playing
	p.mu.Unlock()
	p.bcastProgress()
}

// Seek 定位（web 由前端 audio.currentTime 执行，这里只存状态）
func (p *PlaybackState) Seek(sec float64) {
	p.mu.Lock()
	if sec < 0 {
		sec = 0
	}
	p.cur.Position = sec
	p.mu.Unlock()
	p.bcastProgress()
}

// SetProgress web 位置上报（高频，只更新进度，广播 progress）
func (p *PlaybackState) SetProgress(position, duration float64, playing bool) {
	p.mu.Lock()
	p.cur.Position = position
	if duration > 0 {
		p.cur.Duration = duration
	}
	p.cur.Playing = playing
	p.mu.Unlock()
	p.bcastProgress()
}

// Next / Prev 队列走位（循环）
func (p *PlaybackState) Next() {
	p.mu.Lock()
	if len(p.cur.Queue) < 2 {
		p.mu.Unlock()
		return
	}
	p.cur.QIdx = (p.cur.QIdx + 1) % len(p.cur.Queue)
	p.applyLocked(p.cur.Queue[p.cur.QIdx])
	p.mu.Unlock()
	p.bcastSnapshot()
}

func (p *PlaybackState) Prev() {
	p.mu.Lock()
	if len(p.cur.Queue) < 2 {
		p.mu.Unlock()
		return
	}
	p.cur.QIdx = (p.cur.QIdx - 1 + len(p.cur.Queue)) % len(p.cur.Queue)
	p.applyLocked(p.cur.Queue[p.cur.QIdx])
	p.mu.Unlock()
	p.bcastSnapshot()
}

// applyLocked 把队列某曲设为当前歌（调用方已持锁）
func (p *PlaybackState) applyLocked(s Song) {
	p.cur.BVID = s.BVID
	p.cur.Title = s.Title
	p.cur.Artist = s.Artist
	p.cur.Cover = s.Cover
	p.cur.Duration = s.Duration
	p.cur.Position = 0
	p.cur.Playing = true
}

// SetMode 播放模式
func (p *PlaybackState) SetMode(m string) {
	switch m {
	case "order", "loop", "single", "random":
	default:
		return
	}
	p.mu.Lock()
	p.cur.PlayMode = m
	p.mu.Unlock()
	p.bcastSnapshot()
}

// SetQueue 设置整个队列（收藏夹播放全部等）
func (p *PlaybackState) SetQueue(list []Song, startIdx int) {
	p.mu.Lock()
	p.cur.Queue = append([]Song(nil), list...)
	if startIdx >= 0 && startIdx < len(p.cur.Queue) {
		p.cur.QIdx = startIdx
		p.applyLocked(p.cur.Queue[startIdx])
	}
	p.mu.Unlock()
	p.bcastSnapshot()
}

// SwitchSpeaker 切音箱播放（source=speaker，记录 did）
func (p *PlaybackState) SwitchSpeaker(did string) {
	p.mu.Lock()
	p.cur.Source = "speaker"
	p.cur.SpkDID = did
	p.mu.Unlock()
	p.bcastSnapshot()
}

// SwitchWeb 切回 web
func (p *PlaybackState) SwitchWeb() {
	p.mu.Lock()
	p.cur.Source = "web"
	p.cur.SpkDID = ""
	p.mu.Unlock()
	p.bcastSnapshot()
}

// RemoveQueue 从队列删除（bvid 匹配），删当前则自动切下一首
func (p *PlaybackState) RemoveQueue(bvid string) {
	p.mu.Lock()
	idx := -1
	for i, q := range p.cur.Queue {
		if q.BVID == bvid {
			idx = i
			break
		}
	}
	if idx < 0 {
		p.mu.Unlock()
		return
	}
	p.cur.Queue = append(p.cur.Queue[:idx], p.cur.Queue[idx+1:]...)
	if idx < p.cur.QIdx {
		p.cur.QIdx--
	} else if idx == p.cur.QIdx {
		if len(p.cur.Queue) > 0 {
			p.cur.QIdx = idx % len(p.cur.Queue)
			p.applyLocked(p.cur.Queue[p.cur.QIdx])
		} else {
			p.cur.QIdx = -1
			p.cur.BVID = ""
			p.cur.Title = ""
			p.cur.Artist = ""
			p.cur.Cover = ""
			p.cur.Playing = false
			p.cur.Position = 0
		}
	}
	p.mu.Unlock()
	p.bcastSnapshot()
}

// ClearQueue 清空队列（保留当前歌继续播）
func (p *PlaybackState) ClearQueue() {
	p.mu.Lock()
	p.cur.Queue = []Song{}
	p.cur.QIdx = -1
	p.mu.Unlock()
	p.bcastSnapshot()
}

// NextBVID 当前歌的下一首 bvid（播完自动连播用）
func (p *PlaybackState) NextBVID() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.cur.Queue) < 2 || p.cur.QIdx < 0 {
		return ""
	}
	return p.cur.Queue[(p.cur.QIdx+1)%len(p.cur.Queue)].BVID
}

// SetDuration 设置时长（歌词/元数据到齐后更新）
func (p *PlaybackState) SetDuration(d float64) {
	p.mu.Lock()
	p.cur.Duration = d
	p.mu.Unlock()
	p.bcastProgress()
}

// SetSpeakerProgress 音箱进度上报（后端桥：SpeakerService 1s tick → 统一状态流）。
// 音箱模式（source=speaker 且 did 匹配）才同步状态；否则仅广播进度供前端参考。
func (p *PlaybackState) SetSpeakerProgress(did, bvid, title string, position, duration float64, playing bool) {
	p.mu.Lock()
	isSpk := p.cur.Source == "speaker" && p.cur.SpkDID == did
	if isSpk && bvid != "" {
		p.cur.BVID = bvid
		if title != "" {
			p.cur.Title = title
		}
		p.cur.Position = position
		if duration > 0 {
			p.cur.Duration = duration
		}
		p.cur.Playing = playing
	}
	p.mu.Unlock()
	if isSpk {
		p.bcastProgress()
	}
}

// SetSpeakerStopped 音箱播完/停止：音箱模式切回 web（若 source=speaker 且 did 匹配）
func (p *PlaybackState) SetSpeakerStopped(did string) {
	p.mu.Lock()
	if p.cur.Source == "speaker" && p.cur.SpkDID == did {
		p.cur.Playing = false
		p.cur.Position = 0
	}
	p.mu.Unlock()
	p.bcastProgress()
}
