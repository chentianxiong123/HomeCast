// Package playback 播放域统一状态源（SSOT）：业务状态（当前歌/播放/进度/队列/模式/来源）
// 物理唯一住 Go 后端。前端只发命令（POST /api/v1/state/command）+ 订阅一条 SSE
// （/api/v1/state/stream）渲染。消除 web/音箱/后端 3 处状态硬拼接。
//
// 全链路：入口 internal/api/state.go；消费方前端（面板/歌词/字幕/挂件）订阅同一流；表：state（持久化，阶段4）
package playback

import (
	"database/sql"
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
	db   *sql.DB // 持久化（settings kv），可选注入
	spk  SpeakerController // 音箱控制（可选注入；音箱让位模式下真调音箱 API）
}

// SpkPlayReq 音箱投送请求（playback 包自包含，避免 → speaker 循环依赖）
type SpkPlayReq struct {
	Title    string
	Duration float64
	NextBVID string
	Quality  int
}

// SpeakerController 音箱控制接口（避免 playback → speaker 循环依赖；server 层适配器注入）。
// 音箱让位模式下 Pause/Resume/Next/Prev 真调音箱 API（不只是改本地状态）。
type SpeakerController interface {
	// Control 音箱控制（pause/stop/resume/play）
	Control(did, action string, volume int) (map[string]any, error)
	// Play 投送歌曲到音箱（切歌时重新投下一曲）
	Play(bvid, did string, req SpkPlayReq) (map[string]any, error)
	// Seek 跳转（音箱模式：改内存 offset + 重下发 URL）
	Seek(did string, sec float64) (map[string]any, error)
}

// NewPlaybackState 构造
func NewPlaybackState() *PlaybackState {
	return &PlaybackState{
		cur: State{Source: "web", PlayMode: "order", QIdx: -1, Queue: []Song{}},
		subs: make(map[chan SSEMsg]struct{}),
	}
}

// SetDB 注入持久化句柄（变更即 Save）
func (p *PlaybackState) SetDB(db *sql.DB) {
	p.mu.Lock()
	p.db = db
	p.mu.Unlock()
}

// SetSpeaker 注入音箱控制（server 层调；音箱让位模式真调音箱 API）
func (p *PlaybackState) SetSpeaker(s SpeakerController) {
	p.mu.Lock()
	p.spk = s
	p.mu.Unlock()
}

// spkActive 音箱让位模式生效（source=speaker 且有 did 且已注入控制器）
func (p *PlaybackState) spkActive() (SpeakerController, string) {
	if p.cur.Source == "speaker" && p.cur.SpkDID != "" && p.spk != nil {
		return p.spk, p.cur.SpkDID
	}
	return nil, ""
}
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

// bcastSnapshot 广播全量快照（state 事件）+ 持久化
func (p *PlaybackState) bcastSnapshot() {
	s := p.Snapshot()
	b, _ := json.Marshal(s)
	p.broadcast(SSEMsg{Name: "state", Data: string(b)})
	p.persistNow()
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

// persistNow 立即持久化（若已注入 db）
func (p *PlaybackState) persistNow() {
	p.mu.Lock()
	db := p.db
	p.mu.Unlock()
	if db == nil {
		return
	}
	b := p.persist()
	if _, err := db.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, stateKey, string(b)); err != nil {
		log.Printf("[playback] save state: %v", err)
	}
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
	did := p.cur.SpkDID
	isSpk := p.cur.Source == "speaker" && did != ""
	p.mu.Unlock()
	if isSpk {
		if spk, _ := p.spkActive(); spk != nil {
			if _, err := spk.Control(did, "pause", 0); err != nil {
				log.Printf("[playback] 音箱 pause 失败: %v", err)
			}
		}
	}
	p.bcastProgress()
}

// Resume 恢复
func (p *PlaybackState) Resume() {
	p.mu.Lock()
	p.cur.Playing = true
	did := p.cur.SpkDID
	isSpk := p.cur.Source == "speaker" && did != ""
	p.mu.Unlock()
	if isSpk {
		if spk, _ := p.spkActive(); spk != nil {
			if _, err := spk.Control(did, "resume", 0); err != nil {
				log.Printf("[playback] 音箱 resume 失败: %v", err)
			}
		}
	}
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
	did := p.cur.SpkDID
	isSpk := p.cur.Source == "speaker" && did != ""
	p.mu.Unlock()
	if isSpk {
		// 音箱模式：真 seek（改内存 offset + 重下发 URL 触发音箱重拉）
		if spk, _ := p.spkActive(); spk != nil {
			if _, err := spk.Seek(did, sec); err != nil {
				log.Printf("[playback] 音箱 seek 失败: %v", err)
			}
		}
	}
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
	song := p.cur.Queue[p.cur.QIdx]
	did := p.cur.SpkDID
	isSpk := p.cur.Source == "speaker" && did != ""
	nextBVID := ""
	if len(p.cur.Queue) > 1 {
		nextBVID = p.cur.Queue[(p.cur.QIdx+1)%len(p.cur.Queue)].BVID
	}
	p.mu.Unlock()
	if isSpk {
		// 音箱模式：真投下一曲到音箱（重新 play_by_url）
		if spk, _ := p.spkActive(); spk != nil {
			_, _ = spk.Play(song.BVID, did, SpkPlayReq{
				Title: song.Title, Duration: song.Duration, NextBVID: nextBVID, Quality: 30280,
			})
		}
	}
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
	song := p.cur.Queue[p.cur.QIdx]
	did := p.cur.SpkDID
	isSpk := p.cur.Source == "speaker" && did != ""
	nextBVID := ""
	if len(p.cur.Queue) > 1 {
		nextBVID = p.cur.Queue[(p.cur.QIdx+1)%len(p.cur.Queue)].BVID
	}
	p.mu.Unlock()
	if isSpk {
		if spk, _ := p.spkActive(); spk != nil {
			_, _ = spk.Play(song.BVID, did, SpkPlayReq{
				Title: song.Title, Duration: song.Duration, NextBVID: nextBVID, Quality: 30280,
			})
		}
	}
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

// ClearQueue 清空队列（含当前歌——队列空则无歌可播）
func (p *PlaybackState) ClearQueue() {
	p.mu.Lock()
	p.cur.Queue = []Song{}
	p.cur.QIdx = -1
	p.cur.BVID = ""
	p.cur.Title = ""
	p.cur.Artist = ""
	p.cur.Cover = ""
	p.cur.Playing = false
	p.cur.Position = 0
	p.cur.Duration = 0
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

// ---- 持久化（settings kv，key=playback_state）：存队列/播放模式/上次歌，重启恢复 ----

// stateKey settings 表键
const stateKey = "playback_state"

// persist 序列化持久态（队列/模式/当前歌，不含 position/playing——播放态不跨重启）
func (p *PlaybackState) persist() []byte {
	s := p.Snapshot()
	v := map[string]any{
		"play_mode": s.PlayMode,
		"qidx":      s.QIdx,
		"queue":     s.Queue,
	}
	b, _ := json.Marshal(v)
	return b
}

// Save 持久化当前队列/模式到 settings（变更后调用；失败仅日志，不阻塞播放）
// 已由 bcastSnapshot → persistNow 自动调用，此处保留兼容导出（供外部主动存）
func (p *PlaybackState) Save(db *sql.DB) {
	p.persistNow()
}

// Restore 启动恢复：读 settings 回队列/模式/当前歌（playing=false，不自动播）。
// 无持久化数据则保留默认（Source=web, PlayMode=order, 空队列）。
func (p *PlaybackState) Restore(db *sql.DB) {
	if db == nil {
		return
	}
	var v string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key=?`, stateKey).Scan(&v); err != nil {
		return // 无历史，保留默认
	}
	var d struct {
		PlayMode string `json:"play_mode"`
		QIdx     int    `json:"qidx"`
		Queue    []Song `json:"queue"`
	}
	if json.Unmarshal([]byte(v), &d) != nil {
		return
	}
	p.mu.Lock()
	p.cur.PlayMode = d.PlayMode
	if d.PlayMode == "" {
		p.cur.PlayMode = "order"
	}
	if len(d.Queue) > 0 {
		p.cur.Queue = d.Queue
		// 上次歌恢复（qidx 越界钳制）；不自动播
		if d.QIdx >= 0 && d.QIdx < len(d.Queue) {
			p.cur.QIdx = d.QIdx
		} else {
			p.cur.QIdx = 0
		}
		p.cur.BVID = p.cur.Queue[p.cur.QIdx].BVID
		p.cur.Title = p.cur.Queue[p.cur.QIdx].Title
		p.cur.Artist = p.cur.Queue[p.cur.QIdx].Artist
		p.cur.Cover = p.cur.Queue[p.cur.QIdx].Cover
		p.cur.Playing = false
		p.cur.Position = 0
	}
	p.mu.Unlock()
	if len(d.Queue) > 0 {
		log.Printf("[playback] restore queue=%d qidx=%d cur=%s mode=%s", len(d.Queue), p.Snapshot().QIdx, p.Snapshot().BVID, p.Snapshot().PlayMode)
	}
}
