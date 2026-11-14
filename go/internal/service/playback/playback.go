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
	// manualSwitchAt 主动切歌（Next/Prev）时刻：projectProgress 的防污染窗口用。
	// 主动切歌时序 = 核心先换新歌+bcastSnapshot，后 out.Play 音箱才换——中间窗口
	// progressLoop tick 会读到音箱还播旧歌，若一律跟随会把核心刷回旧歌（前端歌名/进度
	// 错乱，真机踩过）。窗口内且音箱 bvid 滞后 → 跳过投影等音箱 startProgress 跟上。
	// 蓝牙跟随（音箱领先、不经 Next/Prev）与自动连播（EndedHook→Next，音箱同步换歌）不受影响。
	manualSwitchAt time.Time
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
	// 输出已被替换（SwitchWeb/SwitchSpeaker 已切走）：本次投影过期，放弃。
	// 竞态：progressLoop 锁外快照 out 后、这里拿锁前可能已切 web，若继续投影
	// 会把核心刷回音箱旧状态（source=web 但 bvid=旧歌/playing=true）→ 前端
	// driveAudio 依此加载旧歌开始播 = 双响（真机踩过同类）。
	if p.cur.Source != "speaker" || p.out != out {
		p.mu.Unlock()
		return
	}
	if info.BVID != "" && p.cur.BVID != info.BVID {
		// 主动切歌保护窗（3s）：Next/Prev 后音箱还没 startProgress 换歌，其 Position()
		// 仍报旧歌。若跟随会把核心 bvid/title/qidx 刷回旧歌 = 切歌错乱。窗口内跳过本次投影。
		if time.Since(p.manualSwitchAt) < 3*time.Second {
			p.mu.Unlock()
			return
		}
		// 非主动切歌窗口的 bvid 不一致 = 音箱领先（蓝牙/外部换歌）→ 核心跟随设备
		if idx := songIndexLocked(p.cur.Queue, info.BVID); idx >= 0 {
			p.cur.QIdx = idx
		}
		p.cur.BVID = info.BVID
	}
	if info.Title != "" && p.cur.Title != info.Title {
		p.cur.Title = info.Title
	}
	// 音箱已无活动进度（播完→SpeakerFinished / 手动停）：不投影空态覆盖核心。
	// 否则下面 !p.cur.Playing 条件成立会无条件把 position 刷 0（播完显示跳到 0:00），
	// 且每 1s 空广播——progressLoop 对已停音箱的无意义临访。
	if info.BVID == "" {
		p.mu.Unlock()
		return
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
		Playing:  p.cur.Playing,
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

// SpeakerFinished 音箱播完且无下一首（后端 SpeakerService.progressTimer 到点、
// EndedHook(did, "")）：音箱物理已静音，核心落停播态——playing=false、
// 队列/当前歌保留（用户可再播/切歌），不切回 web（音箱仍在首选输出）。
// 旧实现：无此通知，单曲播完核心永远 “playing”、本地时钟虚涨到几倍 duration。
func (p *PlaybackState) SpeakerFinished(did string) {
	p.mu.Lock()
	if p.cur.Source != "speaker" || p.cur.SpkDID != did {
		p.mu.Unlock()
		return // 不是这台音箱在播（已切走/已停）：忽略
	}
	p.cur.Playing = false
	p.mu.Unlock()
	p.bcastSnapshot()
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

// SetProgress web 位置上报（高频，只更新进度，广播 progress）。
// 仅 source=web 时接受：音箱模式的核心进度唯一写者是 projectProgress（音箱投影），
// 若此处也写 = 双写（多客户端/切模式瞬间的旧页面 timeupdate 会用 web 位置
// 覆盖音箱真实进度，同 bvid 时 bvid 校验拦不住——状态脏的结构根源）。
func (p *PlaybackState) SetProgress(position, duration float64, playing bool) {
	p.mu.Lock()
	if p.cur.Source != "web" {
		p.mu.Unlock()
		return // 音箱模式：进度由 projectProgress 单写，web 上报一律拒绝（不污染）
	}
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
	p.cur.Playing = snap.Playing
}

// songSnapshot 切换前状态快照（音箱投送失败回滚用）
type songSnapshot struct {
	QIdx     int
	Song     Song
	Position float64
	Duration float64
	Playing  bool // 切歌前播放态（回滚必须恢复：applyLocked 会置 Playing=true）
}

// curSongLocked 当前歌快照（QIdx 越界安全：-1/空队列 → 空 Song，不 panic）
func (p *PlaybackState) curSongLocked() Song {
	if p.cur.QIdx >= 0 && p.cur.QIdx < len(p.cur.Queue) {
		return p.cur.Queue[p.cur.QIdx]
	}
	return Song{}
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

// Next / Prev 队列走位（循环）。QIdx=-1（空队列未播过）→ 无当前歌可走，return。
func (p *PlaybackState) Next() {
	p.mu.Lock()
	if p.cur.QIdx < 0 || len(p.cur.Queue) < 2 {
		p.mu.Unlock()
		return
	}
	old := songSnapshot{
		QIdx:     p.cur.QIdx,
		Song:     p.curSongLocked(),
		Position: p.cur.Position,
		Duration: p.cur.Duration,
		Playing:  p.cur.Playing,
	}
	p.cur.QIdx = (p.cur.QIdx + 1) % len(p.cur.Queue)
	song := p.cur.Queue[p.cur.QIdx]
	p.applyLocked(song)
	p.manualSwitchAt = time.Now() // 主动切歌：开防污染窗（projectProgress 跳过滞后旧歌投影）
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
	if p.cur.QIdx < 0 || len(p.cur.Queue) < 2 {
		p.mu.Unlock()
		return
	}
	old := songSnapshot{
		QIdx:     p.cur.QIdx,
		Song:     p.curSongLocked(),
		Position: p.cur.Position,
		Duration: p.cur.Duration,
		Playing:  p.cur.Playing,
	}
	p.cur.QIdx = (p.cur.QIdx - 1 + len(p.cur.Queue)) % len(p.cur.Queue)
	song := p.cur.Queue[p.cur.QIdx]
	p.applyLocked(song)
	p.manualSwitchAt = time.Now() // 主动切歌：开防污染窗
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

// SetQueue 设置整个队列（收藏夹/搜索“全部播放”）：
// 与 Play/Next/Prev 同构——锁定内快照（含队列副本：失败必须整体回滚）→ 改状态
// → 解锁后调 out.Play 真投当前输出（音箱模式核心与音箱同步切歌，不再“核心播 B 音箱唱 A")。
// 队列长度 < 2 时不出声（音箱无歌可连播；web 模式照常渲染）。
func (p *PlaybackState) SetQueue(list []Song, startIdx int) {
	p.mu.Lock()
	old := songSnapshot{
		QIdx:     p.cur.QIdx,
		Song:     p.curSongLocked(),
		Position: p.cur.Position,
		Duration: p.cur.Duration,
		Playing:  p.cur.Playing,
	}
	oldQueue := append([]Song(nil), p.cur.Queue...)
	p.cur.Queue = append([]Song(nil), list...)
	if startIdx >= 0 && startIdx < len(p.cur.Queue) {
		p.cur.QIdx = startIdx
		p.applyLocked(p.cur.Queue[startIdx])
	} else {
		p.cur.QIdx = -1
		p.cur.BVID = ""
		p.cur.Playing = false
		p.cur.Position = 0
	}
	nextBVID := p.nextBVIDLocked()
	var playSong Song
	playOK := false
	if p.cur.QIdx >= 0 && p.cur.QIdx < len(p.cur.Queue) {
		playSong = p.cur.Queue[p.cur.QIdx] // 锁内拷贝（解锁后 out.Play 用，勿锁外读）
		playOK = true
	}
	out := p.out
	p.mu.Unlock()
	if out != nil && playSong.BVID != "" {
		if playOK {
			// 队列有歌：真投当前输出（音箱模式核心与音箱同步切歌）
			if err := out.Play(playSong.BVID, PlayOpts{Title: playSong.Title, Duration: playSong.Duration, NextBVID: nextBVID, Quality: 30280}); err != nil {
				log.Printf("[playback] queue 投送失败 bvid=%s err=%v → 回滚继续播旧歌", playSong.BVID, err)
				p.mu.Lock()
				p.cur.Queue = oldQueue
				p.rollbackLocked(old)
				p.mu.Unlock()
			}
		} else {
			// 队列无有效 startIdx（空列表/越界）：清状态后让输出闭嘴（同 ClearQueue 语义）
			if err := out.Stop(); err != nil && err != ErrUnsupported {
				log.Printf("[playback] queue 空队列停输出失败: %v", err)
			}
		}
	}
	p.bcastSnapshot()
}

// SwitchSpeaker 切音箱播放（source=speaker，记录 did，激活对应输出）。
// 注意入径语义：hx/speaker/play **真投成功后才调本函数**（失败不切）——
// 所以核心 Playing 必须立即=true（音箱已在响）；沿用 prevWebPlaying 会谎报
// 暂停 1s（web 切走时暂停 → 音箱在播核心 says paused，等 projectProgress 校准）。
// prevWebPlaying 仍留给 SwitchWeb 续播用（切回 web 恢复 web 侧播放态）。
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
	p.cur.Playing = true // 已真投成功：音箱在响，核心必须同步（勿沿用旧播放态撒谎）
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

// RemoveQueue 从队列删除（bvid 匹配）。删当前歌：自动切下一首并真投当前输出
// （音箱模式下核心与音箱同步；失败回滚继续播旧歌）。删到队列空：真停输出。
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
	old := songSnapshot{
		QIdx:     p.cur.QIdx,
		Song:     p.curSongLocked(),
		Position: p.cur.Position,
		Duration: p.cur.Duration,
		Playing:  p.cur.Playing,
	}
	oldQueue := append([]Song(nil), p.cur.Queue...)
	p.cur.Queue = append(p.cur.Queue[:idx], p.cur.Queue[idx+1:]...)
	var (
		needPlay bool
		playSong Song
		nextBVID string
	)
	if idx < p.cur.QIdx {
		p.cur.QIdx-- // 删当前歌之前的：当前歌不变，仅下标前移
	} else if idx == p.cur.QIdx {
		if len(p.cur.Queue) > 0 {
			// 删当前歌：自动切下一首（核心+输出同步真投）
			p.cur.QIdx = idx % len(p.cur.Queue)
			p.applyLocked(p.cur.Queue[p.cur.QIdx])
			needPlay = true
			playSong = p.cur.Queue[p.cur.QIdx]
			nextBVID = p.nextBVIDLocked()
		} else {
			// 删到空：清状态 + 停输出
			p.cur.QIdx = -1
			p.cur.BVID = ""
			p.cur.Title = ""
			p.cur.Artist = ""
			p.cur.Cover = ""
			p.cur.Playing = false
			p.cur.Position = 0
			p.cur.Duration = 0
		}
	}
	out := p.out
	p.mu.Unlock()
	if needPlay && out != nil {
		if err := out.Play(playSong.BVID, PlayOpts{Title: playSong.Title, Duration: playSong.Duration, NextBVID: nextBVID, Quality: 30280}); err != nil {
			log.Printf("[playback] remove 切下一首失败 bvid=%s err=%v → 回滚继续播旧歌", playSong.BVID, err)
			p.mu.Lock()
			p.cur.Queue = oldQueue
			p.rollbackLocked(old)
			p.mu.Unlock()
		}
	} else if len(p.cur.Queue) == 0 && out != nil {
		// 删到空：让当前输出闭嘴
		if err := out.Stop(); err != nil && err != ErrUnsupported {
			log.Printf("[playback] remove 停输出失败: %v", err)
		}
	}
	p.bcastSnapshot()
}

// ClearQueue 清空队列（含当前歌——队列空则无歌可播）：真停当前输出
// （音箱模式核心清空但音箱继续唱旧歌=状态撒谎，同 RemoveQueue 同族修复）。
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
	out := p.out
	p.mu.Unlock()
	if out != nil {
		if err := out.Stop(); err != nil && err != ErrUnsupported {
			log.Printf("[playback] clear 停输出失败: %v", err)
		}
	}
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
