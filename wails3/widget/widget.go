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
// 数据流：前端 WebView 上报 /widget/state → 后端内存 → 本窗口 SSE 订阅推送（去轮询）；
// 歌词走内嵌后端 /music/lyric?bvid= （已选源优先，与歌词页同步）。
//
// 显示层多态（Display 接口）：linux=GTK4(display_linux.go)、windows=Win32(控制条待补)、
// 安卓=悬浮窗 Service(待)——核心逻辑全在这个文件，显示层只填空。
package widget

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
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
	StatePoll = 900  // ms（SSE 断开重连间隔）
	FontSize  = 30.0
	TickEvery = 150 // ms，主循环周期
)

// SnapshotProvider 播放状态快照（desktop main 提供，直读后端内存）
type SnapshotProvider func() (bvid, title, artist string, currentTime, duration float64, playing bool)

type state struct {
	mu sync.Mutex

	currentLine string // 后端 SSE 算好的当前行（多平台统一：显示层纯渲染）
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
	winW     int
}

var (
	st       state
	prov     SnapshotProvider
	posPath  string
	Backend  string
	disp     Display
	coreTick func()
)

// Start 在宿主壳的界面线程调用（wails OnStartup 时机）
func Start(snapshot SnapshotProvider, backend string) error {
	prov = snapshot
	Backend = backend
	home, _ := os.UserHomeDir()
	posPath = filepath.Join(home, ".config", "homecast", "widget.json")
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
	go listenEvents() // SSE 订阅：状态/切歌即时推送（去 900ms 轮询）
	// 强制首绘：等 GTK 窗口 realized 后重绘（否则 draw_func 只在初始空跑一次）
	time.AfterFunc(1200*time.Millisecond, func() { queueDraw() })
	return nil
}

// MainLoop 进入显示层事件循环（阻塞）。Linux 版 GTK 主循环由宿主壳（wails）驱动，
// 此函数为空操作；Windows/安卓版由各自显示层实现（demo/真壳调用）
func MainLoop() {
	if disp != nil {
		disp.MainLoop()
	}
}

// SetVisible 桌面挂件整个窗口显示/隐藏（下面面板 #d-tick → POST /api/v1/widget/lyric-visible）
func SetVisible(on bool) {
	if disp != nil {
		disp.SetVisible(on)
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
// PlayerCtl 事件桥已退役（前端 htmx 不监听 wails 事件）；±0.5s 仍走本地 offset 显示微调。

func onPauseClick() {
	// 播放/暂停：经后端控制意图中转 → 前端轮询 /api/v1/widget/command 消费
	// （挂件是独立窗口，要控制 WebView 里的 audio 只能走 HTTP；延迟 = 前端 1s 轮询）
	if Backend == "" {
		return
	}
	go func() {
		req, err := http.NewRequest(http.MethodPost, Backend+"/api/v1/widget/toggle", nil)
		if err != nil {
			return
		}
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
		}
	}()
}

func onMinusClick() {
	postOffset(-0.5)
}

func onPlusClick() {
	postOffset(0.5)
}

// postOffset 用户 ±0.5s 微调：改后端持久化 offset（逻辑住后端：PickLine 用 ct+offset 算行），
// 后端 OnOffsetChange → 清缓存重拉 → SSE 广播带新 line → 显示层刷新。桌面/安卓/ web 同源。
func postOffset(d float64) {
	st.mu.Lock()
	st.offset += d
	cur := st.offset
	st.mu.Unlock()
	if Backend == "" {
		return
	}
	st.mu.Lock()
	bvid := st.songKey
	st.mu.Unlock()
	if bvid == "" {
		return
	}
	go func() {
		body := strings.NewReader(`{"bvid":"` + bvid + `","offset":` + fmt.Sprintf("%.1f", cur) + `}`)
		req, err := http.NewRequest(http.MethodPost, Backend+"/api/v1/lyric/offset", body)
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
		}
	}()
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

	// 2. 播放状态由 SSE 推送（listenEvents → applyState），本循环只做悬停/拖拽；
	// 歌词行由后端 SSE 算好（applyState 更新 currentLine → queueDraw），这里不再本地定位。
}

// ── 共享绘制（显示层 goOnDraw → paint(c Canvas)） ──

// 共享绘制（显示层 goOnDraw → paint(c Canvas)）
// 歌词居中基准：窗口宽在切歌时一次性定好（fixLyricWidth），播放中不变 →
// 歌词始终围绕固定中线居中，不逐行左右晃动（对齐 web 字幕面板固定宽）

func paint(c Canvas) {
	st.mu.Lock()
	text := displayTextLocked()
	winW := st.winW
	st.mu.Unlock()

	c.Clear()
	if text == "" {
		return
	}
	w, h := c.TextSize(text, FontSize)
	x := (float64(winW) - w) / 2
	if x < 0 {
		x = 0
	}
	y0 := (float64(WinH) - h) / 2
	if y0 < 0 {
		y0 = 0
	}
	// 阴影 + 主字（白字黑阴影，对齐原版）
	c.Text(x+2, y0+2, text, FontSize, 0, 0, 0, 0.78)
	c.Text(x, y0, text, FontSize, 1, 1, 1, 1)
}


// displayTextLocked 需持锁调用
func displayTextLocked() string {
	// 多平台统一：显示层纯渲染（逻辑住后端），直接用 SSE 算好的当前行
	if st.currentLine != "" {
		return st.currentLine
	}
	if st.title != "" {
		return st.title
	}
	return ""
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
// 对齐 web lyric.js：带 bvid（后端优先用「已选歌词源」——切源后两端同步同一份歌词）
// + 代际保护（切歌时旧请求晚到丢弃，对齐 web loadGen）+ 本歌存档对齐偏移（meta 端点）

// applyState SSE 推送的播放状态落地（多平台统一：显示层纯渲染，不本地定位）
// 逻辑一份住后端（PickLine）；这里只用 SSE 算好的 line。切歌清行，同歌更新 line。
func applyState(bvid, title, artist, line string, ct, dur float64, playing bool) {
	st.mu.Lock()
	if st.songKey != bvid {
		st.songKey = bvid
		st.title = joinTitle(title, artist)
		st.currentLine = ""
		st.dur = dur
		st.playing = playing
		st.winW = MinW
	}
	st.playing = playing
	st.dur = dur
	st.currentLine = line // 后端算好的当前行（唯一逻辑产物）
	st.mu.Unlock()
	queueDraw() // 状态变了立即重绘（GTK4 draw_func 需显式 queue）
}

// listenEvents 订阅后端 SSE（/api/v1/widget/events），事件到达即刷新歌词状态。
// 断线按 StatePoll 间隔重连；这是挂件唯一的播放状态来源（不再轮询快照）。
func listenEvents() {
	for {
		if Backend == "" {
			time.Sleep(StatePoll * time.Millisecond)
			continue
		}
		req, err := http.NewRequest("GET", Backend+"/api/v1/widget/events", nil)
		if err != nil {
			time.Sleep(StatePoll * time.Millisecond)
			continue
		}
		req.Header.Set("Accept", "text/event-stream")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			time.Sleep(StatePoll * time.Millisecond)
			continue
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			time.Sleep(StatePoll * time.Millisecond)
			continue
		}
		// 先拉一次当前快照，避免等首个事件前空白（line 空，SSE 后续带真值）
		if prov != nil {
			b, t, a, ct, dur, pl := prov()
			applyState(b, t, a, "", ct, dur, pl)
		}
		readSSE(resp.Body)
		resp.Body.Close()
		time.Sleep(StatePoll * time.Millisecond) // 断开后重连
	}
}

// readSSE 解析 event:/data: 行，遇 lyric-state 即 applyState
func readSSE(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	var event string
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if event == "lyric-state" && data != "" {
				var p struct {
					BVID        string  `json:"bvid"`
					Title       string  `json:"title"`
					Artist      string  `json:"artist"`
					CurrentTime float64 `json:"current_time"`
					Duration    float64 `json:"duration"`
					Playing     bool    `json:"playing"`
					Line        string  `json:"line"`
				}
				if json.Unmarshal([]byte(data), &p) == nil {
					applyState(p.BVID, p.Title, p.Artist, p.Line, p.CurrentTime, p.Duration, p.Playing)
				}
			}
			event = ""
		case line == "":
			event = ""
		}
	}
}
