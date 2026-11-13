package playback

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
)

func TestPlayQueueAndNext(t *testing.T) {
	p := NewPlaybackState()
	p.Play(Song{BVID: "A", Title: "a"})
	p.Play(Song{BVID: "B", Title: "b"})
	s := p.Snapshot()
	if len(s.Queue) != 2 || s.QIdx != 1 || s.BVID != "B" {
		t.Fatalf("Play queue wrong: q=%d idx=%d cur=%s", len(s.Queue), s.QIdx, s.BVID)
	}
	// 去重：再播 A → 移尾
	p.Play(Song{BVID: "A", Title: "a2"})
	s = p.Snapshot()
	if len(s.Queue) != 2 || s.QIdx != 1 || s.BVID != "A" || s.Queue[0].BVID != "B" {
		t.Fatalf("dedupe wrong: q=%d idx=%d cur=%s first=%s", len(s.Queue), s.QIdx, s.BVID, s.Queue[0].BVID)
	}
	// next 循环
	p.Next()
	s = p.Snapshot()
	if s.BVID != "B" {
		t.Fatalf("next wrong: %s", s.BVID)
	}
	p.Next()
	s = p.Snapshot()
	if s.BVID != "A" {
		t.Fatalf("next loop wrong: %s", s.BVID)
	}
}

func TestToggleSeekMode(t *testing.T) {
	p := NewPlaybackState()
	p.Play(Song{BVID: "A"})
	p.Toggle()
	if p.Snapshot().Playing {
		t.Fatal("toggle should pause")
	}
	p.Resume()
	if !p.Snapshot().Playing {
		t.Fatal("resume should play")
	}
	p.Seek(42)
	if p.Snapshot().Position != 42 {
		t.Fatalf("seek: %v", p.Snapshot().Position)
	}
	p.SetMode("random")
	if p.Snapshot().PlayMode != "random" {
		t.Fatal("mode")
	}
}

func TestSwitchSource(t *testing.T) {
	p := NewPlaybackState()
	p.Play(Song{BVID: "A"})
	p.SwitchSpeaker("did123")
	s := p.Snapshot()
	if s.Source != "speaker" || s.SpkDID != "did123" {
		t.Fatalf("switch speaker: %+v", s)
	}
	p.SwitchWeb()
	s = p.Snapshot()
	if s.Source != "web" || s.SpkDID != "" {
		t.Fatalf("switch web: %+v", s)
	}
}

func TestRemoveClearQueue(t *testing.T) {
	p := NewPlaybackState()
	p.SetQueue([]Song{{BVID: "A"}, {BVID: "B"}, {BVID: "C"}}, 0)
	if p.Snapshot().QIdx != 0 || p.Snapshot().BVID != "A" {
		t.Fatalf("setqueue: %+v", p.Snapshot())
	}
	p.RemoveQueue("A")
	s := p.Snapshot()
	if len(s.Queue) != 2 || s.BVID != "B" {
		t.Fatalf("remove current wrong: q=%d cur=%s", len(s.Queue), s.BVID)
	}
	p.ClearQueue()
	s = p.Snapshot()
	if len(s.Queue) != 0 || s.BVID != "" {
		t.Fatalf("clear: q=%d bvid=%s", len(s.Queue), s.BVID)
	}
}

func TestSubscribeBroadcast(t *testing.T) {
	p := NewPlaybackState()
	ch := p.Subscribe()
	defer p.Unsubscribe(ch)
	p.Play(Song{BVID: "A"})
	select {
	case m := <-ch:
		if m.Name != "state" {
			t.Fatalf("first event should be state, got %s", m.Name)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast")
	}
	// progress 事件
	p.SetProgress(10, 100, true)
	select {
	case m := <-ch:
		if m.Name != "progress" {
			t.Fatalf("second event should be progress, got %s", m.Name)
		}
	case <-time.After(time.Second):
		t.Fatal("no progress broadcast")
	}
}

func TestNextBVID(t *testing.T) {
	p := NewPlaybackState()
	p.SetQueue([]Song{{BVID: "A"}, {BVID: "B"}}, 0)
	if p.NextBVID() != "B" {
		t.Fatal("next bvid")
	}
	p.Next()
	if p.NextBVID() != "A" {
		t.Fatal("next bvid loop")
	}
}

func TestPersistRestore(t *testing.T) {
	// 临时 SQLite
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT)`); err != nil {
		t.Fatal(err)
	}
	p := NewPlaybackState()
	p.SetDB(db)
	p.SetQueue([]Song{{BVID: "A", Title: "a"}, {BVID: "B", Title: "b"}}, 1)
	p.SetMode("random")
	p.Save(db) // 主动触发（SetQueue/SetMode 经 bcastSnapshot 已自动存，此处显式保底）
	// 新实例恢复
	p2 := NewPlaybackState()
	p2.Restore(db)
	s := p2.Snapshot()
	if len(s.Queue) != 2 || s.QIdx != 1 || s.BVID != "B" || s.PlayMode != "random" {
		t.Fatalf("restore wrong: queue=%d qidx=%d cur=%s mode=%s", len(s.Queue), s.QIdx, s.BVID, s.PlayMode)
	}
	if s.Playing {
		t.Fatal("restore should not auto-play")
	}
}

// mockOutput 记录 Play 调用,可模拟投送失败
type mockOutput struct {
	playCalls []string
	failOn    string // 匹配到该 bvid 时返回错误
}

func (m *mockOutput) Play(bvid string, opts PlayOpts) error {
	m.playCalls = append(m.playCalls, bvid)
	if m.failOn == bvid {
		return errors.New("mock fail")
	}
	return nil
}
func (m *mockOutput) Pause() error                { return nil }
func (m *mockOutput) Resume() error               { return nil }
func (m *mockOutput) Stop() error                 { return nil }
func (m *mockOutput) Seek(sec float64) error      { return nil }
func (m *mockOutput) SetVolume(v int) error       { return nil }
func (m *mockOutput) Position() ProgressInfo         { return ProgressInfo{} }
func (m *mockOutput) Caps() Capabilities          { return Capabilities{Play: true} }
func (m *mockOutput) SetEndedHook(func())         {}
func (m *mockOutput) Name() string                { return "mock" }

// 音箱模式下 Play 必须真投当前输出(收藏夹/搜索点歌=切歌);失败回滚继续播旧歌
func TestPlayCallsOutputAndRollback(t *testing.T) {
	mock := &mockOutput{}
	p := NewPlaybackState()
	p.SetOutput(mock)
	p.Play(Song{BVID: "A", Title: "a"}) // source 保持 web,Null 语义;但 out 是 mock → 也投
	if len(mock.playCalls) != 1 || mock.playCalls[0] != "A" {
		t.Fatalf("Play 未调用输出: %v", mock.playCalls)
	}
	// 投送 B 失败 → 回滚到 A
	mock.failOn = "B"
	p.Play(Song{BVID: "B", Title: "b"})
	s := p.Snapshot()
	if s.BVID != "A" || s.Title != "a" {
		t.Fatalf("失败未回滚: cur=%s/%s", s.BVID, s.Title)
	}
	if len(s.Queue) != 1 || s.Queue[0].BVID != "A" {
		t.Fatalf("失败后队列错位: %+v", s.Queue)
	}
	// 恢复成功路径:再播 C 成功
	mock.failOn = ""
	p.Play(Song{BVID: "C", Title: "c"})
	s = p.Snapshot()
	if s.BVID != "C" {
		t.Fatalf("成功未切: %s", s.BVID)
	}
}

// speakerProgOut 模拟音箱输出:Position() 带 bvid/title(音箱自报)；
// playFn/stopFn 可注入记录调用/制造失败（验证"真投/真停/回滚"语义）。
type speakerProgOut struct {
	bvid, title string
	pos         float64
	dur         float64
	playing     bool
	playFn      func(bvid string) error
	stopFn      func() error
	lastPlayed  string
}

func (m *speakerProgOut) Play(bvid string, opts PlayOpts) error {
	m.lastPlayed = bvid
	if m.playFn != nil {
		return m.playFn(bvid)
	}
	return nil
}
func (m *speakerProgOut) Pause() error  { return nil }
func (m *speakerProgOut) Resume() error { return nil }
func (m *speakerProgOut) Stop() error {
	if m.stopFn != nil {
		return m.stopFn()
	}
	return nil
}
func (m *speakerProgOut) Seek(sec float64) error { return ErrUnsupported }
func (m *speakerProgOut) SetVolume(v int) error { return nil }
func (m *speakerProgOut) Position() ProgressInfo {
	return ProgressInfo{BVID: m.bvid, Title: m.title, Pos: m.pos, Dur: m.dur, Playing: m.playing}
}
func (m *speakerProgOut) Caps() Capabilities  { return Capabilities{Play: true} }
func (m *speakerProgOut) SetEndedHook(func()) {}
func (m *speakerProgOut) Name() string        { return "speaker" }

// 无额外辅助函数：测试直接读 mock.lastPlayed

// TestProgressLoopProjectsSpeakerSong 重构图核心：progressLoop 全量投影——
// 音箱自报 bvid/title 同步进核心（自动连播/蓝牙切换后核心与设备不分裂）。
// 替代已删的 SetSpeakerProgress 桥（当时无生产调用=死代码，语义迁到此处）。
func TestProgressLoopProjectsSpeakerSong(t *testing.T) {
	out := &speakerProgOut{bvid: "BV1x", title: "音箱自唱歌", pos: 30, dur: 200, playing: true}
	p := NewPlaybackState()
	p.Play(Song{BVID: "core-old", Title: "核心旧歌"})
	p.SwitchSpeaker("did1")
	p.SetOutput(out)

	// 直接调一次投影逻辑（progressLoop 内部等价路径，不依赖 1s 时钟）
	p.projectProgress(out)
	s := p.Snapshot()
	if s.BVID != "BV1x" || s.Title != "音箱自唱歌" {
		t.Fatalf("核心未投影音箱当前歌: %+v", s)
	}
	if s.Position != 30 || s.Duration != 200 || !s.Playing || s.Source != "speaker" {
		t.Fatalf("核心未同步音箱进度: %+v", s)
	}
}

// TestSetQueuePlaysOutputAndRollsBack 收藏夹/搜索"播放全部"音箱模式必须真投：
// 旧实现 SetQueue 只改核心队列/当前歌，从不调 out.Play——核心说播 B 音箱唱 A
// （与 Play 老 bug 同族，真机踩过"放着歌一切换队列就不动"）。
func TestSetQueuePlaysOutputAndRollsBack(t *testing.T) {
	var called string
	var fail bool
	out := &speakerProgOut{bvid: "BV-old", pos: 10, dur: 100, playing: true}
	out.playFn = func(bvid string) error {
		called = bvid
		if fail {
			return errors.New("boom")
		}
		return nil
	}
	p := NewPlaybackState()
	p.Play(Song{BVID: "BV-old", Title: "旧歌"})
	p.SwitchSpeaker("did1")
	p.SetOutput(out)

	// 播放全部：新队列 B/C/D，qidx=0 → 音箱必须真投 B
	p.SetQueue([]Song{{BVID: "BV-B", Title: "B"}, {BVID: "BV-C", Title: "C"}}, 0)
	if called != "BV-B" {
		t.Fatalf("SetQueue 未真投输出: called=%q", called)
	}
	if s := p.Snapshot(); s.BVID != "BV-B" || s.QIdx != 0 || !s.Playing {
		t.Fatalf("核心未切换到新队列: %+v", s)
	}

	// 投送失败 → 回滚旧队列+当前歌+播放态（含 Playing：暂停态不能回滚成播放中）
	fail = true
	p.Pause() // 先暂停（Playing=false）
	p.SetQueue([]Song{{BVID: "BV-X", Title: "X"}}, 0)
	s := p.Snapshot()
	if s.BVID != "BV-B" || s.Playing {
		t.Fatalf("SetQueue 失败未回滚旧状态(含播放态): %+v", s)
	}
	if len(s.Queue) != 2 || s.Queue[0].BVID != "BV-B" {
		t.Fatalf("SetQueue 失败未回滚队列: %+v", s.Queue)
	}
}

// TestRemoveQueueCurrentSongPlaysNext 删当前歌必须真投下一首（音箱模式核心与音箱同步）。
func TestRemoveQueueCurrentSongPlaysNext(t *testing.T) {
	out := &speakerProgOut{bvid: "BV-A", pos: 5, dur: 100, playing: true}
	out.playFn = func(bvid string) error { return nil }
	p := NewPlaybackState()
	p.SetQueue([]Song{{BVID: "BV-A"}, {BVID: "BV-B"}}, 0)
	p.SwitchSpeaker("did1")
	p.SetOutput(out)
	out.lastPlayed = "" // 重置：只看 RemoveQueue 的投送

	p.RemoveQueue("BV-A")
	if out.lastPlayed != "BV-B" {
		t.Fatalf("删当前歌未真投下一首: %q", out.lastPlayed)
	}
	if s := p.Snapshot(); s.BVID != "BV-B" || s.QIdx != 0 {
		t.Fatalf("删当前歌核心未切下一首: %+v", s)
	}
}

// TestRemoveQueueToEmptyStopsOutput 删到队列空必须真停输出（音箱不能继续唱被删的歌）。
func TestRemoveQueueToEmptyStopsOutput(t *testing.T) {
	out := &speakerProgOut{bvid: "BV-A", pos: 5, dur: 100, playing: true}
	var stopped bool
	out.stopFn = func() error { stopped = true; return nil }
	p := NewPlaybackState()
	p.SetQueue([]Song{{BVID: "BV-A"}}, 0)
	p.SwitchSpeaker("did1")
	p.SetOutput(out)

	p.RemoveQueue("BV-A")
	s := p.Snapshot()
	if s.BVID != "" || s.Playing {
		t.Fatalf("删到空核心未清空: %+v", s)
	}
	if !stopped {
		t.Fatal("删到空未停输出（音箱继续唱）")
	}
}

// TestProjectProgressSkipsAfterSwitch set output 竞态守卫：progressLoop 投影时
// 输出已被 SwitchWeb 替换 → 本次投影必须放弃（否则 source=web 但 bvid/playing
// 被音箱旧状态覆盖 → 前端 driveAudio 加载旧歌播放 = 双响）。
func TestProjectProgressSkipsAfterSwitch(t *testing.T) {
	out := &speakerProgOut{bvid: "BV-spk", pos: 30, dur: 200, playing: true}
	p := NewPlaybackState()
	p.Play(Song{BVID: "BV-spk", Title: "音箱歌"})
	p.SwitchSpeaker("did1")
	p.SetOutput(out)

	// 模拟：快照 out 后、投影前切回了 web（输出已换 NullOutput）
	p.SwitchWeb()
	p.projectProgress(out) // 旧 out 投影——必须被守卫拦截

	s := p.Snapshot()
	if s.Source != "web" || s.BVID != "BV-spk" {
		t.Fatalf("切回 web 后旧投影污染核心: %+v", s)
	}
}

// TestSetProgressRejectedWhenSpeaker 音箱模式 web 上报必须被拒（唯一写者=projectProgress）：
// 多客户端/切模式瞬间旧页面 timeupdate 覆盖音箱进度 = 状态脏结构根源。
func TestSetProgressRejectedWhenSpeaker(t *testing.T) {
	out := &speakerProgOut{bvid: "BV-spk", pos: 30, dur: 200, playing: true}
	p := NewPlaybackState()
	p.Play(Song{BVID: "BV-spk", Title: "音箱歌"})
	p.SwitchSpeaker("did1")
	p.SetOutput(out)
	p.projectProgress(out)

	p.SetProgress(99, 200, false) // 音箱模式：必须拒绝
	if s := p.Snapshot(); s.Position != 30 || !s.Playing {
		t.Fatalf("音箱模式 web 上报未被拒（双写）: %+v", s)
	}

	p.SwitchWeb()
	p.SetProgress(42, 200, true) // 切回 web 后放行
	if s := p.Snapshot(); s.Position != 42 || !s.Playing {
		t.Fatalf("web 模式上报被误拒: %+v", s)
	}
}
