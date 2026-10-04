// Package widget 桌面歌词挂件——核心逻辑（平台无关）
//
// 特性对齐原版 Qt 挂件（lyric_widget.py）：
//   - 背景像素级透明 + 置顶无边框 + 不抢焦点
//   - X11 真穿透（显示层），悬停 ≥1.2s 浮现控制条并恢复输入
//   - 底部控制条：暂停/±0.5s 微调/锁定/关闭；锁定态悬停只出小锁、不可拖
//   - 单句大字号居中（白字+黑阴影），宽度随文本自适应(240~1500)
//   - 位置记忆 ~/.config/homecast/widget.json；整窗可拖
//   - 播放进度本地平滑推进 + 状态校准
//
// 数据流：前端 WebView 上报 /widget/state → 后端内存 → 本窗口轮询快照；
// 歌词走内嵌后端 /music/lyric?bvid= （已选源优先，与歌词页同步）。
//
// 显示层多态（Display 接口）：linux=GTK3(display_linux.go)、windows=Win32(待)、
// 安卓=悬浮窗 Service(待)——核心逻辑全在这个文件，显示层只填空。
package widget

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ── 配置（对齐原版） ──
const (
	WinH      = 116
	CtrlH     = 24
	MinW      = 240
	MaxW      = 1500
	HoverMs   = 1200 // 悬停浮现控制条的时长
	StatePoll = 900  // ms
	FontSize  = 30.0
	TickEvery = 150 // ms，主循环周期
)

// SnapshotProvider 播放状态快照（desktop main 提供，直读后端内存）
type SnapshotProvider func() (bvid, title, artist string, currentTime, duration float64, playing bool)

// PlayerCtl 播放器控制（Wails EventsEmit → 前端）
type PlayerCtl interface {
	Emit(cmd string, payload ...any)
}

// LyricLine 歌词行
type LyricLine struct {
	Sec  float64
	Text string
}

type state struct {
	mu sync.Mutex

	lines   []LyricLine
	curIdx  int
	pos     float64 // 当前时间（校准值 + 本地推进）
	offset  float64 // 用户微调
	dur     float64
	playing bool
	title   string
	songKey string

	uiShown  bool
	locked   bool
	closed   bool
	hoverT0  int64
	dragDX   int
	dragDY   int
	dragging bool
	lastPoll time.Time
	winW     int
}

var (
	st       state
	prov     SnapshotProvider
	ctl      PlayerCtl
	posPath  string
	Backend  string
	disp     Display
	coreTick func()
)

// Start 在宿主壳的界面线程调用（wails OnStartup 时机）
func Start(snapshot SnapshotProvider, playerCtl PlayerCtl, backend string) error {
	prov = snapshot
	ctl = playerCtl
	Backend = backend
	home, _ := os.UserHomeDir()
	posPath = filepath.Join(home, ".config", "homecast", "widget.json")
	st.curIdx = -1
	st.winW = MinW

	x, y := loadPos()
	d := newDisplay()
	sw, sh := d.ScreenSize()
	// 位置不合法（无记忆/负坐标/超出屏幕——例如 Linux 位置被 Wine 更小屏幕加载）→ 居中
	if x < 0 || y < 0 || x > sw-MinW || y > sh-WinH {
		x = (sw - MinW) / 2
		y = sh - WinH - 40
	}

	disp = newDisplay()
	if err := disp.Init(x, y); err != nil {
		return err
	}
	// 校准：Init 后显示层已建，此时 ScreenSize 才可靠（Wine 等虚拟屏在窗口创建前可能误报）
	px, py := disp.Pos()
	sw, sh = disp.ScreenSize()
	log.Printf("[widget] 位置校准: pos=(%d,%d) screen=%dx%d", px, py, sw, sh)
	if px < 0 || py < 0 || px > sw-MinW || py > sh-WinH {
		px, py = (sw-MinW)/2, sh-WinH-40
		if px < 0 || py < 0 {
			px, py = 0, 0
		}
		disp.Move(px, py)
		log.Printf("[widget] 位置超屏，已居中到 (%d,%d)", px, py)
	}
	disp.Tick(TickEvery, tick)
	return nil
}

// MainLoop 进入显示层事件循环（阻塞）。Linux 版 GTK 主循环由宿主壳（wails）驱动，
// 此函数为空操作；Windows/安卓版由各自显示层实现（demo/真壳调用）
func MainLoop() {
	if disp != nil {
		disp.MainLoop()
	}
}

// ── 位置记忆 ──

func loadPos() (int, int) {
	b, err := os.ReadFile(posPath)
	if err != nil {
		return -1, -1
	}
	var p struct {
		X int `json:"x"`
		Y int `json:"y"`
	}
	if json.Unmarshal(b, &p) != nil {
		return -1, -1
	}
	return p.X, p.Y
}

func savePos(x, y int) {
	b, _ := json.Marshal(map[string]int{"x": x, "y": y})
	if err := os.WriteFile(posPath, b, 0o644); err != nil {
		log.Printf("[widget] 保存位置失败: %v", err)
	}
}

// ── 控制条按钮动作（cglue.c _hc_btn_cb → //export 回调 → 核心） ──

func onPauseClick() {
	if ctl != nil {
		ctl.Emit("toggle")
	}
}

func onMinusClick() {
	if ctl != nil {
		ctl.Emit("seek", -0.5)
	}
	st.mu.Lock()
	st.offset -= 0.5
	st.mu.Unlock()
	queueDraw()
}

func onPlusClick() {
	if ctl != nil {
		ctl.Emit("seek", 0.5)
	}
	st.mu.Lock()
	st.offset += 0.5
	st.mu.Unlock()
	queueDraw()
}

func onLockClick() {
	st.mu.Lock()
	st.locked = !st.locked
	locked := st.locked
	st.mu.Unlock()
	if locked {
		disp.ShowUI(true)
	} else {
		if st.uiShown {
			disp.ShowUI(false)
		} else {
			disp.HideUI()
		}
	}
	log.Printf("[widget] 锁定=%v", locked)
}

func onCloseClick() {
	st.mu.Lock()
	st.closed = true
	st.mu.Unlock()
	disp.Quit()
	log.Printf("[widget] 挂件已关闭")
}

// ── 鼠标交互（显示层事件 → 核心） ──

func handlePress(x, y float64, primary bool) {
	st.mu.Lock()
	if st.locked || st.closed {
		st.mu.Unlock()
		return
	}
	st.mu.Unlock()
	if primary {
		gx, gy := disp.Pos()
		st.mu.Lock()
		st.dragDX = int(x) - gx
		st.dragDY = int(y) - gy
		st.dragging = true
		st.mu.Unlock()
	}
}

func handleMotion(x, y float64) {
	st.mu.Lock()
	if !st.dragging || st.locked || st.closed {
		st.mu.Unlock()
		return
	}
	dx, dy := st.dragDX, st.dragDY
	st.mu.Unlock()
	disp.Move(int(x)-dx, int(y)-dy)
}

func handleRelease() {
	st.mu.Lock()
	st.dragging = false
	st.mu.Unlock()
	gx, gy := disp.Pos()
	savePos(gx, gy)
}

// ── 悬停浮现/收起 ──

func showUI() {
	st.mu.Lock()
	st.uiShown = true
	locked := st.locked
	st.mu.Unlock()
	disp.ShowUI(locked)
}

func hideUI() {
	st.mu.Lock()
	st.uiShown = false
	st.mu.Unlock()
	disp.HideUI()
}

func queueDraw() {
	if disp != nil {
		disp.QueueDraw()
	}
}

// ── 主循环 tick：悬停检测 + 状态同步 + 进度推进（显示层周期调用） ──

func tick() {
	st.mu.Lock()
	if st.closed {
		st.mu.Unlock()
		return
	}
	st.mu.Unlock()

	// 1. 悬停检测
	inside := disp.CursorInside()
	now := time.Now().UnixMilli()
	st.mu.Lock()
	uiShown := st.uiShown
	hoverT0 := st.hoverT0
	st.mu.Unlock()
	if inside {
		if hoverT0 == 0 {
			st.mu.Lock()
			st.hoverT0 = now
			st.mu.Unlock()
		} else if now-hoverT0 >= HoverMs && !uiShown {
			showUI()
		}
	} else {
		st.mu.Lock()
		st.hoverT0 = 0
		st.mu.Unlock()
		if uiShown {
			hideUI()
		}
	}

	// 2. 状态同步（节流）+ 歌词切换 + 平滑推进
	st.mu.Lock()
	pollDue := time.Since(st.lastPoll).Milliseconds() >= StatePoll
	playing := st.playing
	st.mu.Unlock()
	if pollDue {
		st.lastPoll = time.Now()
		bvid, title, artist, ct, dur, playing2 := "", "", "", 0.0, 0.0, false
		if prov != nil {
			bvid, title, artist, ct, dur, playing2 = prov()
		}
		key := bvid
		st.mu.Lock()
		if st.songKey != key {
			st.songKey = key
			st.title = joinTitle(title, artist)
			st.lines = nil
			st.curIdx = -1
			st.pos = 0
			st.offset = 0
			st.dur = dur
			st.playing = playing2
			st.mu.Unlock()
			if key != "" {
				go fetchLyric(joinTitle(title, artist), key)
			}
			st.mu.Lock()
		} else {
			st.playing = playing2
			st.dur = dur
			if !playing2 {
				st.pos = ct // 暂停：以上报为准
			} else if absF(ct-st.pos) > 1.5 {
				st.pos = ct // 校准（seek）
			}
		}
		playing = st.playing
		st.mu.Unlock()
	}

	// 3. 本地平滑推进（播放中每 tick +150ms）
	if playing {
		st.mu.Lock()
		st.pos += float64(TickEvery) / 1000.0
		idx := displayIdxLocked()
		if idx != st.curIdx {
			st.curIdx = idx
			st.mu.Unlock()
			queueDraw()
			return
		}
		st.mu.Unlock()
	}
}

// ── 共享绘制（显示层 goOnDraw → paint(c Canvas)） ──

func paint(c Canvas) {
	st.mu.Lock()
	text := displayTextLocked()
	winW := st.winW
	st.mu.Unlock()

	// 宽度自适应：变化 >24px 才 resize（避免每帧抖动）
	target := targetWidth(text)
	if abs(winW-target) > 24 {
		st.mu.Lock()
		st.winW = target
		st.mu.Unlock()
		if disp != nil {
			disp.Resize(target, WinH)
		}
		return // 下帧重绘
	}

	c.Clear()
	if text == "" {
		return
	}
	winWf := float64(winW)
	w, h := c.TextSize(text, FontSize)
	x := (winWf - w) / 2
	if x < 0 {
		x = 0
	}
	y0 := (WinH - h) / 2
	if y0 < 0 {
		y0 = 0
	}
	// 阴影 + 主字（白字黑阴影，对齐原版）
	c.Text(x+2, y0+2, text, FontSize, 0, 0, 0, 0.78)
	c.Text(x, y0, text, FontSize, 1, 1, 1, 1)
}

// displayTextLocked 需持锁调用
func displayTextLocked() string {
	if len(st.lines) > 0 && st.curIdx >= 0 && st.curIdx < len(st.lines) {
		return st.lines[st.curIdx].Text
	}
	if st.title != "" {
		return st.title
	}
	return ""
}

func displayIdxLocked() int {
	t := st.pos + st.offset
	idx := -1
	for k := range st.lines {
		if st.lines[k].Sec <= t {
			idx = k
		} else {
			break
		}
	}
	return idx
}

func targetWidth(text string) int {
	w := len([]rune(text))*32 + 90
	if w < MinW {
		w = MinW
	}
	if w > MaxW {
		w = MaxW
	}
	return w
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func absF(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func joinTitle(title, artist string) string {
	t := strings.TrimSpace(title)
	a := strings.TrimSpace(artist)
	if a == "" || strings.Contains(t, a) {
		return t
	}
	return t + " " + a
}

// ── 歌词拉取（goroutine + 内嵌后端 HTTP） ──
// 带 bvid：后端命中「已选歌词源」（歌词页切源上报）返回同一份歌词——两端同步

func fetchLyric(keyword, key string) {
	q := url.QueryEscape(keyword)
	u := Backend + "/api/v1/music/lyric?keyword=" + q
	if key != "" {
		u += "&bvid=" + url.QueryEscape(key)
	}
	resp, err := http.Get(u)
	if err != nil {
		log.Printf("[widget] 歌词请求失败: %v", err)
		return
	}
	defer resp.Body.Close()
	var out struct {
		Code int `json:"code"`
		Data *struct {
			Lines [][2]any `json:"lines"`
		} `json:"data"`
	}
	if json.NewDecoder(resp.Body).Decode(&out) != nil || out.Data == nil {
		return
	}
	lines := make([]LyricLine, 0, len(out.Data.Lines))
	for _, l := range out.Data.Lines {
		var line LyricLine
		if len(l) < 2 {
			continue
		}
		if f, ok := l[0].(float64); ok {
			line.Sec = f
		}
		if s, ok := l[1].(string); ok {
			line.Text = s
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return
	}
	st.mu.Lock()
	if st.songKey != key {
		st.mu.Unlock()
		return // 已切歌，丢弃
	}
	st.lines = lines
	st.curIdx = displayIdxLocked()
	st.mu.Unlock()
	queueDraw()
	log.Printf("[widget] 歌词 %d 行 🎵%s", len(lines), keyword)
}