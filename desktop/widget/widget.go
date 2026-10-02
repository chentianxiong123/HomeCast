// Package widget 桌面歌词挂件（cgo + GTK3，移植自 lyric_widget.py）
//
// 特性对齐原版 Qt 挂件：
//   - 背景像素级透明 + 置顶无边框 + 不抢焦点（GTK ARGB visual）
//   - X11 真穿透（XShapeCombineMask），悬停 ≥1.2s 浮现控制条并恢复输入
//   - 底部控制条：暂停/±0.5s 微调/锁定/关闭；锁定态悬停只出小锁、不可拖
//   - 单句大字号居中（白字+黑阴影），宽度随文本自适应(240~1500)
//   - 位置记忆 ~/.config/homecast/widget.json；整窗可拖
//   - 播放进度本地平滑推进 + 状态校准
//
// 数据流：前端 WebView 上报 /widget/state → 后端内存 → 本窗口轮询快照；
// 歌词走内嵌后端 /music/lyric（网易云）。
package widget

/*
#cgo pkg-config: gtk+-3.0
#cgo LDFLAGS: -lX11 -lXext
#include <gtk/gtk.h>
#include <gdk/gdkx.h>
#include <X11/Xlib.h>
#include <X11/extensions/shape.h>

// C 实现（变量/回调/穿透/样式）在 cglue.c，这里只留声明
extern GtkWidget* hc_win(void);
extern GtkWidget* hc_ctrl(void);
extern GtkWidget* hc_lockbtn(void);
extern GtkWidget* hc_fixed(void);
extern GdkWindow* hc_win_gdk(void);
extern void hc_set_win(GtkWidget*);
extern void hc_set_ctrl(GtkWidget*);
extern void hc_set_lockbtn(GtkWidget*);
extern void hc_set_fixed(GtkWidget*);
extern void hc_set_win_gdk(GdkWindow*);
extern void _hc_btn_connect(GtkWidget*, int);
extern void _hc_css(void);
extern void wire_signals(void);
extern void xshape_mask(int);
extern int pointer_global(int*, int*);
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"
)

// ── 配置（对齐原版） ──
const (
	WinH     = 116
	CtrlH    = 24
	MinW     = 240
	MaxW     = 1500
	HoverMs  = 1200 // 悬停浮现控制条的时长
	StatePoll = 900 // ms
	FontDesc = "Microsoft YaHei 30"
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

	lines  []LyricLine
	curIdx int
	pos    float64 // 当前时间（校准值 + 本地推进）
	offset float64 // 用户微调
	dur    float64
	playing bool
	title  string
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
	st      state
	prov    SnapshotProvider
	ctl     PlayerCtl
	posPath string
	Backend string
)

// Start 必须在 GTK 主线程调用（Wails OnStartup 时机）
func Start(snapshot SnapshotProvider, playerCtl PlayerCtl, backend string) error {
	prov = snapshot
	ctl = playerCtl
	Backend = backend
	home, _ := os.UserHomeDir()
	posPath = filepath.Join(home, ".config", "homecast", "widget.json")
	st.curIdx = -1
	st.winW = MinW

	x, y := loadPos()
	if x < 0 || y < 0 {
		// 默认：屏幕水平居中，底部上方 40px（对齐原版）
		sw := int(C.gdk_screen_width())
		sh := int(C.gdk_screen_height())
		x = (sw - MinW) / 2
		y = sh - WinH - 40
	}

	cwin := C.gtk_window_new(C.GTK_WINDOW_TOPLEVEL)
	C.gtk_window_set_decorated((*C.GtkWindow)(unsafe.Pointer(cwin)), 0)
	C.gtk_window_set_keep_above((*C.GtkWindow)(unsafe.Pointer(cwin)), 1)
	C.gtk_window_set_accept_focus((*C.GtkWindow)(unsafe.Pointer(cwin)), 0)
	C.gtk_window_set_type_hint((*C.GtkWindow)(unsafe.Pointer(cwin)), C.GDK_WINDOW_TYPE_HINT_UTILITY)
	C.gtk_widget_set_app_paintable(cwin, 1)
	visual := C.gdk_screen_get_rgba_visual(C.gtk_widget_get_screen(cwin))
	if visual == nil {
		return fmt.Errorf("无 RGBA visual")
	}
	C.gtk_widget_set_visual(cwin, visual)
	C.gtk_window_set_default_size((*C.GtkWindow)(unsafe.Pointer(cwin)), C.gint(MinW), C.gint(WinH))
	C.gtk_window_move((*C.GtkWindow)(unsafe.Pointer(cwin)), C.gint(x), C.gint(y))
	C.gtk_widget_add_events(cwin, C.GDK_BUTTON_PRESS_MASK|C.GDK_BUTTON_RELEASE_MASK|C.GDK_POINTER_MOTION_MASK)

	fixed := C.gtk_fixed_new()
	C.gtk_container_add((*C.GtkContainer)(unsafe.Pointer(cwin)), fixed)
	C.gtk_widget_show(fixed)

	// 底部控制条
	ctrl := C.gtk_box_new(C.GTK_ORIENTATION_HORIZONTAL, 6)
	C.gtk_widget_set_name(ctrl, C.CString("hc-ctrl"))
	C.gtk_widget_set_size_request(ctrl, 0, C.gint(CtrlH))
	mkBtn := func(label string, w, kind int) {
		b := C.gtk_button_new_with_label(C.CString(label))
		C.gtk_widget_set_size_request(b, C.gint(w), C.gint(CtrlH))
		C.gtk_widget_set_name(b, C.CString("hc-btn"))
		C._hc_btn_connect(b, C.int(kind))
		C.gtk_box_pack_start((*C.GtkBox)(unsafe.Pointer(ctrl)), b, 0, 0, 0)
		C.gtk_widget_show(b)
	}
	mkBtn("‖", 24, 1)   // 暂停/播放
	mkBtn("−0.5", 44, 2) // 微调
	mkBtn("+0.5", 44, 3)
	mkBtn("🔓", 28, 4)  // 锁定切换
	mkBtn("×", 22, 5)   // 关闭
	C.gtk_fixed_put((*C.GtkFixed)(unsafe.Pointer(fixed)), ctrl, 0, C.gint(WinH-CtrlH))
	C.gtk_widget_hide(ctrl)

	// 锁定态小锁
	lockBtn := C.gtk_button_new_with_label(C.CString("🔒"))
	C.gtk_widget_set_size_request(lockBtn, C.gint(16), C.gint(16))
	C.gtk_widget_set_name(lockBtn, C.CString("hc-lock"))
	C._hc_btn_connect(lockBtn, C.int(4))
	C.gtk_fixed_put((*C.GtkFixed)(unsafe.Pointer(fixed)), lockBtn, 6, C.gint(WinH-16-6))
	C.gtk_widget_hide(lockBtn)

	C._hc_css()
	C.gtk_widget_show(cwin)

	gw := C.gtk_widget_get_window(cwin)
	C.hc_set_win(cwin)
	C.hc_set_fixed(fixed)
	C.hc_set_ctrl(ctrl)
	C.hc_set_lockbtn(lockBtn)
	C.hc_set_win_gdk(gw)
	C.wire_signals()
	C.xshape_mask(0) // 初始穿透
	st.mu.Lock()
	st.winW = MinW
	st.mu.Unlock()
	log.Printf("[widget] 桌面歌词挂件已启动 (pos=%d,%d)", x, y)
	return nil
}

// ── 位置记忆 ──
func loadPos() (int, int) {
	data, err := os.ReadFile(posPath)
	if err != nil {
		return -1, -1
	}
	var p struct{ X, Y int }
	if json.Unmarshal(data, &p) != nil {
		return -1, -1
	}
	return p.X, p.Y
}

func savePos(x, y int) {
	_ = os.MkdirAll(filepath.Dir(posPath), 0o755)
	b, _ := json.Marshal(map[string]int{"x": x, "y": y})
	_ = os.WriteFile(posPath, b, 0o600)
}

// ── 按钮回调（C → Go） ──
//export goOnPauseClick
func goOnPauseClick() {
	if ctl != nil {
		ctl.Emit("toggle")
	}
}

//export goOnMinusClick
func goOnMinusClick() {
	if ctl != nil {
		ctl.Emit("seek", -0.5)
	}
	st.mu.Lock()
	st.offset -= 0.5
	st.mu.Unlock()
	queueDraw()
}

//export goOnPlusClick
func goOnPlusClick() {
	if ctl != nil {
		ctl.Emit("seek", 0.5)
	}
	st.mu.Lock()
	st.offset += 0.5
	st.mu.Unlock()
	queueDraw()
}

//export goOnLockClick
func goOnLockClick() {
	st.mu.Lock()
	st.locked = !st.locked
	locked := st.locked
	st.mu.Unlock()
	if locked {
		C.gtk_widget_hide(C.hc_ctrl())
		C.gtk_widget_show(C.hc_lockbtn())
	} else {
		C.gtk_widget_show(C.hc_ctrl())
		C.gtk_widget_hide(C.hc_lockbtn())
	}
	log.Printf("[widget] 锁定=%v", locked)
}

//export goOnCloseClick
func goOnCloseClick() {
	st.mu.Lock()
	st.closed = true
	st.mu.Unlock()
	C.gtk_widget_destroy(C.hc_win())
	log.Printf("[widget] 挂件已关闭")
}

// ── 绘制 ──
//export goOnDraw
func goOnDraw(w *C.GtkWidget, cr *C.cairo_t, user unsafe.Pointer) C.gboolean {
	st.mu.Lock()
	text := displayTextLocked()
	winW := st.winW
	st.mu.Unlock()
	target := targetWidth(text)
	if abs(winW-target) > 24 {
		st.mu.Lock()
		st.winW = target
		st.mu.Unlock()
		C.gtk_window_resize((*C.GtkWindow)(unsafe.Pointer(C.hc_win())), C.gint(target), C.gint(WinH))
		C.gtk_widget_queue_draw(C.hc_win())
		return 0
	}

	// 透明底
	C.cairo_set_source_rgba(cr, 0, 0, 0, 0)
	C.cairo_paint(cr)
	if text == "" {
		return 1
	}

	layout := C.pango_cairo_create_layout(cr)
	desc := C.pango_font_description_from_string(C.CString(FontDesc))
	C.pango_layout_set_font_description(layout, desc)
	cs := C.CString(text)
	C.pango_layout_set_text(layout, cs, -1)
	var pw, ph C.gint
	C.pango_layout_get_size(layout, &pw, &ph)
	pxW := int(pw) / 1024
	x := (winW - pxW) / 2
	if x < 0 {
		x = 0
	}
	// 垂直居中：用 ink extents（文字实际墨水框，不含行距），
	// 之前用基线公式会把字画到窗口底部被裁剪
	var ink C.PangoRectangle
	C.pango_layout_get_pixel_extents(layout, &ink, nil)
	y0 := (WinH - int(ink.height)) / 2
	if y0 < 0 {
		y0 = 0
	}
	// 阴影
	C.cairo_set_source_rgba(cr, 0, 0, 0, 0.78)
	C.cairo_move_to(cr, C.gdouble(x+2), C.gdouble(y0+2))
	C.pango_cairo_show_layout(cr, layout)
	// 主字
	C.cairo_set_source_rgba(cr, 1, 1, 1, 1)
	C.cairo_move_to(cr, C.gdouble(x), C.gdouble(y0))
	C.pango_cairo_show_layout(cr, layout)
	C.g_object_unref(C.gpointer(unsafe.Pointer(layout)))
	C.pango_font_description_free(desc)
	C.free(unsafe.Pointer(cs))
	return 1
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

func queueDraw() {
	st.mu.Lock()
	closed := st.closed
	st.mu.Unlock()
	if !closed && C.hc_win() != nil {
		C.gtk_widget_queue_draw(C.hc_win())
	}
}

// ── 拖拽（悬停浮现期间整窗可拖；锁定态不可） ──
//export goOnButtonPress
func goOnButtonPress(w *C.GtkWidget, e *C.GdkEventButton, u unsafe.Pointer) C.gboolean {
	st.mu.Lock()
	if st.locked || st.closed {
		st.mu.Unlock()
		return 1
	}
	st.mu.Unlock()
	if e.button == 1 {
		var gx, gy C.gint
		C.gtk_window_get_position((*C.GtkWindow)(unsafe.Pointer(C.hc_win())), &gx, &gy)
		st.mu.Lock()
		st.dragDX = int(e.x_root) - int(gx)
		st.dragDY = int(e.y_root) - int(gy)
		st.dragging = true
		st.mu.Unlock()
	}
	return 1
}

//export goOnButtonRelease
func goOnButtonRelease(w *C.GtkWidget, e *C.GdkEventButton, u unsafe.Pointer) C.gboolean {
	st.mu.Lock()
	st.dragging = false
	st.mu.Unlock()
	var gx, gy C.gint
	C.gtk_window_get_position((*C.GtkWindow)(unsafe.Pointer(C.hc_win())), &gx, &gy)
	savePos(int(gx), int(gy))
	return 1
}

//export goOnMotion
func goOnMotion(w *C.GtkWidget, e *C.GdkEventMotion, u unsafe.Pointer) C.gboolean {
	st.mu.Lock()
	if !st.dragging || st.locked || st.closed {
		st.mu.Unlock()
		return 1
	}
	dx, dy := st.dragDX, st.dragDY
	st.mu.Unlock()
	C.gtk_window_move((*C.GtkWindow)(unsafe.Pointer(C.hc_win())),
		C.gint(int(e.x_root)-dx), C.gint(int(e.y_root)-dy))
	return 1
}

// ── 主循环 tick：悬停检测 + 状态同步 + 进度推进 ──
//export goTick
func goTick(user unsafe.Pointer) C.gboolean {
	st.mu.Lock()
	if st.closed {
		st.mu.Unlock()
		return 0 // 停止 timeout
	}
	st.mu.Unlock()

	// 1. 悬停检测
	inside := false
	var px, py C.int
	if C.pointer_global(&px, &py) == 1 {
		var gx, gy C.gint
		C.gtk_window_get_position((*C.GtkWindow)(unsafe.Pointer(C.hc_win())), &gx, &gy)
		ww := int(C.gtk_widget_get_allocated_width(C.hc_win()))
		hh := int(C.gtk_widget_get_allocated_height(C.hc_win()))
		inside = int(px) >= int(gx) && int(px) <= int(gx)+ww &&
			int(py) >= int(gy) && int(py) <= int(gy)+hh
	}
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
			return 1
		}
		st.mu.Unlock()
	}
	return 1
}

const TickEvery = 150

func joinTitle(title, artist string) string {
	t := strings.TrimSpace(title)
	a := strings.TrimSpace(artist)
	if a == "" || strings.Contains(t, a) {
		return t
	}
	return t + " " + a
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

// ── 歌词拉取（goroutine + 内嵌后端 HTTP） ──
func fetchLyric(keyword, key string) {
	q := url.QueryEscape(keyword)
	resp, err := http.Get(Backend + "/api/v1/music/lyric?keyword=" + q)
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
		sec, _ := l[0].(float64)
		text, _ := l[1].(string)
		lines = append(lines, LyricLine{Sec: sec, Text: text})
	}
	st.mu.Lock()
	if st.songKey != key {
		st.mu.Unlock()
		return
	}
	st.lines = lines
	st.curIdx = displayIdxLocked()
	st.mu.Unlock()
	queueDraw()
	log.Printf("[widget] 歌词 %d 行 🎵%s", len(lines), keyword)
}

// ── UI 显示/隐藏 ──
func showUI() {
	st.mu.Lock()
	st.uiShown = true
	locked := st.locked
	st.mu.Unlock()
	if locked {
		C.gtk_widget_hide(C.hc_ctrl())
		C.gtk_widget_show(C.hc_lockbtn())
	} else {
		C.gtk_widget_show(C.hc_ctrl())
		C.gtk_widget_hide(C.hc_lockbtn())
	}
	C.xshape_mask(1)
}

func hideUI() {
	st.mu.Lock()
	st.uiShown = false
	st.mu.Unlock()
	C.gtk_widget_hide(C.hc_ctrl())
	C.gtk_widget_hide(C.hc_lockbtn())
	C.xshape_mask(0)
}