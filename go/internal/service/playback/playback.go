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
	"time"
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
	// out 当前激活的播放后端（web=NullOutput 前端 audio 接管；speaker=SpeakerOutput；cast=CastOutput）
	// 统一调度：核心只认 Output 接口，不再用 source 分支判断（对接归位）
	out        Output
	outFactory OutputFactory // 创建绑定 did 的输出（SwitchSpeaker 用）
	prevWebPlaying bool // 切走前 web 播放态（SwitchWeb 续播用，非持久化）
}

// NewPlaybackState 构造
func NewPlaybackState() *PlaybackState {
	p := &PlaybackState{
		cur:  State{Source: "web", PlayMode: "order", QIdx: -1, Queue: []Song{}},
		subs: make(map[chan SSEMsg]struct{}),
		out:  NullOutput{}, // 默认 web：前端 audio 接管
	}
	go p.progressLoop() // 后台：音箱本地模拟进度 → SSOT（一切以我为准，不查音箱）
	return p
}

// progressLoop 进度轮询（1s tick）：source=speaker 时读当前输出本地模拟进度 → SSOT。
// web 模式进度由前端 timeupdate 上报（SetProgress），音箱由这里本地模拟——核心统一调度。
func (p *PlaybackState) progressLoop() {
	tick := time.NewTicker(1 * time.Second)
	defer tick.Stop()
	for range tick.C {
		p.mu.Lock()
		src := p.cur.Source
		out := p.out
		p.mu.Unlock()
		if src != "speaker" || out == nil {
			continue
		}
		p.projectProgress(out)
	}
}

// projectProgress 音箱进度全量投影到核心（唯一入口，测试复用）：
// init —— 进度 + 音箱自报当前歌（bvid/title）→ 核心对齐。
// 音箱自动连播/蓝牙切换后核心 bvid/title 与设备同步（源不分裂），
// bvid 若在队列中同步 QIdx（队头高亮对齐，Next 从正确位置走位）。
// 位置防回零校验：只信 playing 或非零位置的来源（偶发 0 不殓正在播的进度）。
func (p *PlaybackState) projectProgress(out Output) {
	info := out.Position()
	p.mu.Lock()
	if info.BVID != "" {
		if p.cur.BVID != info.BVID {
			if idx := songIndexLocked(p.cur.Queue, info.BVID); idx >= 0 {
				p.cur.QIdx = idx
			}
			p.cur.BVID = info.BVID
		}
		if info.Title != "" && p.cur.Title != info.Title {
			p.cur.Title = info.Title
		}
	}
	// 偶发 position=0 上报不殓正在播的进度（review 低能 #5 防殪）
	if info.Playing || info.Pos > 0 || !p.cur.Playing {
		p.cur.Position = info.Pos
	}
	if info.Dur > 0 {
		p.cur.Duration = info.Dur
	}
	p.cur.Playing = info.Playing
	p.mu.Unlock()
	p.bcastProgress()
}

// SetDB 注入持久化句柄（变更即 Save）
func (p *PlaybackState) SetDB(db *sql.DB) {
	p.mu.Lock()
	p.db = db
	p.mu.Unlock()
}

// SetOutputFactory 注入输出工厂（server 层调）：SwitchSpeaker(did) 时创建绑定该 did 的输出
// （SpeakerOutput/CastOutput），实现"对接归位"——核心只认 Output，不再 source 分支判断。
type OutputFactory func(did string) Output

func (p *PlaybackState) SetOutputFactory(f OutputFactory) {
	p.mu.Lock()
	p.outFactory = f
	p.mu.Unlock()
}

// outActive 当前激活输出（无则 NullOutput——web 模式前端 audio 接管）
func (p *PlaybackState) outActive() Output {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.out == nil {
		return NullOutput{}
	}
	return p.out
}

// SetOutput 直接设当前输出（测试/特殊场景用）
func (p *PlaybackState) SetOutput(out Output) {
	p.mu.Lock()
	p.out = out
	p.mu.Unlock()
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

// Play 播放某歌：入队（去重）+ 定位 + 播（source 保持；web=Null 前端 audio 接管，speaker=真投音箱）
func (p *PlaybackState) Play(s Song) {
	p.mu.Lock()
	// 快照（含队列副本：Play 会去重+入队尾，失败必须整体回滚，只还原 QIdx 会导致元素错位）
	old := songSnapshot{
		QIdx: p.cur.QIdx,
		Song: Song{BVID: p.cur.BVID, Title: p.cur.Title, Artist: p.cur.Artist, Cover: p.cur.Cover, Duration: p.cur.Duration},
		Position: p.cur.Position,
		Duration: p.cur.Duration,
	}
	oldQueue := append([]Song(nil), p.cur.Queue...)
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
	nextBVID := p.nextBVIDLocked()
	out := p.out
	p.mu.Unlock()
	log.Printf("[playback] Play bvid=%s source=%s", s.BVID, p.Snapshot().Source)
	// 当前输出真切歌（web=Null 前端重载；speaker=真投新歌；失败回滚继续播旧歌）
	if out != nil {
		if err := out.Play(s.BVID, PlayOpts{Title: s.Title, Duration: s.Duration, NextBVID: nextBVID, Quality: 30280}); err != nil {
			log.Printf("[playback] play 投送失败 bvid=%s err=%v → 回滚继续播旧歌", s.BVID, err)
			p.mu.Lock()
			p.cur.Queue = oldQueue
			p.rollbackLocked(old)
			p.mu.Unlock()
		}
	}
	p.bcastSnapshot()
}

// Pause 暂停（统一：本地状态 + 当前输出真暂停。web=Null 前端 audio 接管；speaker=真调音箱）
func (p *PlaybackState) Pause() {
	p.mu.Lock()
	p.cur.Playing = false
	out := p.out
	p.mu.Unlock()
	if err := out.Pause(); err != nil && err != ErrUnsupported {
		log.Printf("[playback] pause: %v", err)
	}
	p.bcastSnapshot()
}

// Resume 恢复（统一：本地状态 + 当前输出真恢复）
func (p *PlaybackState) Resume() {
	p.mu.Lock()
	p.cur.Playing = true
	out := p.out
	p.mu.Unlock()
	if err := out.Resume(); err != nil && err != ErrUnsupported {
		log.Printf("[playback] resume: %v", err)
	}
	p.bcastSnapshot()
}

// Toggle 播放/暂停切换（统一：本地状态 + 当前输出）
func (p *PlaybackState) Toggle() {
	p.mu.Lock()
	p.cur.Playing = !p.cur.Playing
	target := p.cur.Playing
	out := p.out
	p.mu.Unlock()
	if target {
		if err := out.Resume(); err != nil && err != ErrUnsupported {
			log.Printf("[playback] resume: %v", err)
		}
	} else {
		if err := out.Pause(); err != nil && err != ErrUnsupported {
			log.Printf("[playback] pause: %v", err)
		}
	}
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
	needPersist := false
	if duration > 0 {
		p.cur.Duration = duration
		// 回写 queue 里当前歌的真实时长（音箱模式切歌用 queue[k].Duration，必须准确）
		if p.cur.QIdx >= 0 && p.cur.QIdx < len(p.cur.Queue) && p.cur.Queue[p.cur.QIdx].Duration != duration {
			p.cur.Queue[p.cur.QIdx].Duration = duration
			needPersist = true
		}
	}
	p.cur.Playing = playing
	p.mu.Unlock()
	if needPersist {
		p.persistNow() // 锁已释放，安全调（内部自己加锁）
	}
	p.bcastProgress()
}

// rollbackLocked 回滚到旧状态（调用方持锁）：恢复队列位置+当前歌+进度+播放态，
// 避免音箱投送失败后 UI/后端/音箱三方分裂（review #1：回滚不完整会把进度清零闪一下子）。
func (p *PlaybackState) rollbackLocked(snap songSnapshot) {
	p.cur.QIdx = snap.QIdx
	p.applyLocked(snap.Song)
	p.cur.Position = snap.Position
	p.cur.Duration = snap.Duration
}

// songSnapshot 切换前状态快照（音箱投送失败回滚用）
type songSnapshot struct {
	QIdx     int
	Song     Song
	Position float64
	Duration float64
}

// songIndexLocked 找 bvid 在队列中的位置（-1 未找到；调用方持锁）
func songIndexLocked(q []Song, bvid string) int {
	for i, s := range q {
		if s.BVID == bvid {
			return i
		}
	}
	return -1
}

// Next / Prev 队列走位（循环）
func (p *PlaybackState) Next() {
	p.mu.Lock()
	if len(p.cur.Queue) < 2 {
		p.mu.Unlock()
		return
	}
	old := songSnapshot{
		QIdx:     p.cur.QIdx,
		Song:     p.cur.Queue[p.cur.QIdx],
		Position: p.cur.Position,
		Duration: p.cur.Duration,
	}
	p.cur.QIdx = (p.cur.QIdx + 1) % len(p.cur.Queue)
	song := p.cur.Queue[p.cur.QIdx]
	p.applyLocked(song)
	nextBVID := p.nextBVIDLocked()
	out := p.out
	p.mu.Unlock()
	// 当前输出真切歌（web=Null 前端重载；speaker=真投下一曲；失败回滚继续播旧歌）
	if out != nil {
		if err := out.Play(song.BVID, PlayOpts{Title: song.Title, Duration: song.Duration, NextBVID: nextBVID, Quality: 30280}); err != nil {
			log.Printf("[playback] next 投送失败 bvid=%s err=%v → 回滚继续播旧歌", song.BVID, err)
			p.mu.Lock()
			p.rollbackLocked(old)
			p.mu.Unlock()
		}
	}
	p.bcastSnapshot()
}

// nextBVIDLocked 当前 QIdx 的下一首 bvid（循环；需持锁）
func (p *PlaybackState) nextBVIDLocked() string {
	if len(p.cur.Queue) < 2 {
		return ""
	}
	return p.cur.Queue[(p.cur.QIdx+1)%len(p.cur.Queue)].BVID
}

func (p *PlaybackState) Prev() {
	p.mu.Lock()
	if len(p.cur.Queue) < 2 {
		p.mu.Unlock()
		return
	}
	old := songSnapshot{
		QIdx:     p.cur.QIdx,
		Song:     p.cur.Queue[p.cur.QIdx],
		Position: p.cur.Position,
		Duration: p.cur.Duration,
	}
	p.cur.QIdx = (p.cur.QIdx - 1 + len(p.cur.Queue)) % len(p.cur.Queue)
	song := p.cur.Queue[p.cur.QIdx]
	p.applyLocked(song)
	nextBVID := p.nextBVIDLocked()
	out := p.out
	p.mu.Unlock()
	// 当前输出真切上一曲（web=Null 前端重载；speaker=真投上一曲）
	if out != nil {
		if err := out.Play(song.BVID, PlayOpts{Title: song.Title, Duration: song.Duration, NextBVID: nextBVID, Quality: 30280}); err != nil {
			log.Printf("[playback] prev 投送失败 bvid=%s err=%v → 回滚继续播旧歌", song.BVID, err)
			p.mu.Lock()
			p.rollbackLocked(old)
			p.mu.Unlock()
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

// SwitchSpeaker 切音箱播放（source=speaker，记录 did，激活对应输出）
func (p *PlaybackState) SwitchSpeaker(did string) {
	p.mu.Lock()
	prevOut := p.out
	// 同一台音箱重复投送：前端已先真投（SpeakerService 内部强制停旧+播新），
	// 此处若再停旧输出会误停刚投的歌——仅当 did 变化（web→音箱 / 换台音箱）才停旧设备
	sameDevice := p.cur.Source == "speaker" && p.cur.SpkDID == did
	// 记住切走前 web 的播放态：切回 web 时续播同一首（音箱在播→切回 web 继续播）
	p.prevWebPlaying = p.cur.Playing
	p.cur.Source = "speaker"
	p.cur.SpkDID = did
	// 清进度：切模式瞬间不残留 web 旧 position/duration，等音箱本地模拟进度推新值覆盖
	p.cur.Position = 0
	p.cur.Duration = 0
	// 对接归位：创建绑定该 did 的输出（SpeakerOutput），核心后续只认 Output
	if p.outFactory != nil {
		p.out = p.outFactory(did)
	}
	p.mu.Unlock()
	// 独占：换设备（另一台音箱）必须让旧设备闭嘴——锁外真网络调用
	if prevOut != nil && !sameDevice {
		prevOut.Stop()
	}
	p.bcastSnapshot()
}

// StopSpeaker 设备页“停止”：若该 did 正是当前音箱 → 停音箱 + 回 web（手机按投送前状态续播）；
// 非当前设备 → no-op（它本来就没在播，无状态可扰）。
func (p *PlaybackState) StopSpeaker(did string) {
	p.mu.Lock()
	active := p.cur.Source == "speaker" && p.cur.SpkDID == did
	p.mu.Unlock()
	if active {
		p.SwitchWeb() // 停旧输出(音箱) + source=web + 按 prevWebPlaying 续播
	}
}

// SwitchWeb 切回 web（source=web，输出回 Null——前端 audio 接管）
func (p *PlaybackState) SwitchWeb() {
	p.mu.Lock()
	prevOut := p.out
	p.cur.Source = "web"
	p.cur.SpkDID = ""
	// 切回 web：等浏览器加载后由 loadedmetadata/progress 推真实值，先清避免残留音箱旧进度
	p.cur.Position = 0
	p.cur.Duration = 0
	// 续播：沿用切走前 web 的播放态（切走时在播→切回继续播；切走时暂停→切回暂停）
	// 前端 driveAudio 在切回 web 时会强制重载源，p.playing=true → audio.play() 真续播
	p.cur.Playing = p.prevWebPlaying
	p.out = NullOutput{} // web：前端 audio 接管，核心不再调输出
	p.mu.Unlock()
	// 独占：回 web 前必须让旧输出闭嘴（音箱仍在播=双响）
	if prevOut != nil {
		prevOut.Stop()
	}
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
