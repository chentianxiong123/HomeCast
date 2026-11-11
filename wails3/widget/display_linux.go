//go:build linux

// Linux 显示层：cgo + GTK4 + X11 真穿透（照 GTK3 版逻辑迁移，布局/行为一致）
package widget

/*
#cgo pkg-config: gtk4
#cgo LDFLAGS: -lX11 -lXext
#include <gtk/gtk.h>
#include <gdk/x11/gdkx.h>
#include <X11/Xlib.h>
#include <X11/extensions/shape.h>

// C 实现（变量/回调/穿透/样式）在 cglue.c，这里只留声明
extern GtkWidget* hc_win(void);
extern GtkWidget* hc_ctrl(void);
extern GtkWidget* hc_lockbtn(void);
extern GtkWidget* hc_fixed(void);
extern void hc_set_win(GtkWidget*);
extern void hc_set_visible(int);
extern void hc_set_ctrl(GtkWidget*);
extern void hc_set_lockbtn(GtkWidget*);
extern void hc_set_fixed(GtkWidget*);
extern void hc_set_draw_area(GtkWidget*);
extern GtkWidget* hc_draw(void);
extern void _hc_btn_connect(GtkWidget*, int);
extern void hc_set_play_icon(int);
extern void hc_set_playbtn(GtkWidget*);
extern GtkWidget* hc_playbtn(void);
extern void _hc_css(void);
extern void wire_signals(void);
extern void xshape_mask(int);
extern int pointer_global(int*, int*);
extern void hc_move(int, int);
extern void hc_resize(int, int);
extern void hc_get_pos(int*, int*);
extern void hc_keep_above(void);
extern void hc_ctrl_fullwidth(int);
extern void hc_bg_on(void);
extern void hc_bg_off(void);
extern void _init_cb(void);
*/
import "C"

import (
	"time"
	"fmt"
	"log"
	"unsafe"
)

// gtkDisplay Linux GTK4 显示层实现（照 GTK3 版逻辑）
type gtkDisplay struct{}

func newDisplay() Display { return &gtkDisplay{} }

// GTK4 严格主线程：Init 经 g_idle_add 调度到 GTK 主循环（wails 主线程）
var initDone chan error
var initX, initY int

func (d *gtkDisplay) Init(x, y int) error {
	initX, initY = x, y
	initDone = make(chan error, 1)
	C._init_cb()
	return <-initDone
}

//export goInit
func goInit() {
	initDone <- (&gtkDisplay{}).initGtk4(initX, initY)
}

func (d *gtkDisplay) initGtk4(x, y int) error {
	cwin := C.gtk_window_new()
	C.gtk_window_set_decorated((*C.GtkWindow)(unsafe.Pointer(cwin)), 0)
	C.gtk_window_set_default_size((*C.GtkWindow)(unsafe.Pointer(cwin)), C.gint(MinW), C.gint(WinH))
	// 浮层语义：不占任务栏/Alt-Tab/抢焦点 + 无条件置顶。GTK4 已删 set_keep_above /
	// set_type_hint / skip_taskbar，故全走 X11 原生（cglue.c hc_keep_above 增强：
	// 设置 _NET_WM_WINDOW_TYPE_UTILITY + _NET_WM_STATE ABOVE|SKIP_TASKBAR|SKIP_PAGER，
	// 并在窗口 map 事件后由 WM 接管时重发，kwin 才可靠生效）

	fixed := C.gtk_fixed_new()
	C.gtk_window_set_child((*C.GtkWindow)(unsafe.Pointer(cwin)), fixed)

	// 绘制区（GTK4 无窗口 draw，用 GtkDrawingArea + draw_func）
	da := C.gtk_drawing_area_new()
	C.hc_set_draw_area(da)
	C.gtk_widget_set_size_request(da, C.gint(MinW), C.gint(WinH))
	C.gtk_fixed_put((*C.GtkFixed)(unsafe.Pointer(fixed)), da, 0, 0)

	// 底部控制条：外层全宽容器（宽度跟踪窗口，见 Resize），内层用中心盒居中按钮组
	ctrl := C.gtk_box_new(C.GTK_ORIENTATION_HORIZONTAL, 6)
	C.gtk_widget_set_name(ctrl, C.CString("hc-ctrl"))
	C.gtk_widget_set_size_request(ctrl, C.gint(MinW), C.gint(CtrlH)) // 初始全宽，Resize 时跟踪窗口宽
	// 内层：中心盒（center-box），子按钮组水平居中于全宽 ctrl
	center := C.gtk_center_box_new()
	C.gtk_widget_set_hexpand(center, 1) // 关键：center 填满 ctrl 全宽，内部 btns 才居中
	// 5 个按钮用横向 box 包一组作为 center 内容
	btns := C.gtk_box_new(C.GTK_ORIENTATION_HORIZONTAL, 6)
	C.gtk_center_box_set_center_widget((*C.GtkCenterBox)(unsafe.Pointer(center)), btns)
	mkBtn := func(b ctrlBtn) {
		bb := C.gtk_button_new_with_label(C.CString(b.Label))
		C.gtk_widget_set_size_request(bb, C.gint(b.W), C.gint(CtrlH))
		C.gtk_widget_set_name(bb, C.CString("hc-btn"))
		C._hc_btn_connect(bb, C.int(b.Kind))
		if b.Kind == 1 { // 播放/暂停按钮：存全局引用供图标切换
			C.hc_set_playbtn(bb)
		}
		C.gtk_box_append((*C.GtkBox)(unsafe.Pointer(btns)), bb)
	}
	for _, b := range ctrlBtnTable() {
		mkBtn(b)
	}
	C.gtk_box_append((*C.GtkBox)(unsafe.Pointer(ctrl)), center)
	// 外层 ctrl 放左下角，宽度由 Resize 跟踪（全宽 → center-box 自动居中按钮组）
	C.gtk_fixed_put((*C.GtkFixed)(unsafe.Pointer(fixed)), ctrl, 0, C.double(WinH-CtrlH))
	C.gtk_widget_set_visible(ctrl, 0)

	// 锁定态小锁（放大到 18，更醒目）
	lockBtn := C.gtk_button_new_with_label(C.CString("🔒"))
	C.gtk_widget_set_size_request(lockBtn, C.gint(18), C.gint(18))
	C.gtk_widget_set_name(lockBtn, C.CString("hc-lock"))
	C._hc_btn_connect(lockBtn, C.int(4))
	C.gtk_fixed_put((*C.GtkFixed)(unsafe.Pointer(fixed)), lockBtn, 6, C.double(WinH-18-6))
	C.gtk_widget_set_visible(lockBtn, 0)

	C.hc_set_win(cwin)
	C.hc_set_fixed(fixed)
	C.hc_set_ctrl(ctrl)
	C.hc_set_lockbtn(lockBtn)
	C._hc_css()
	C.gtk_widget_set_visible(cwin, 1)
	C.wire_signals()
	C.hc_keep_above()
	// KDE/kwin 需窗口完全 map 后重发 _NET_WM_STATE（Alt+Tab 不显示挂件）
	time.AfterFunc(800*time.Millisecond, func() { C.hc_keep_above() })
	time.AfterFunc(2000*time.Millisecond, func() { C.hc_keep_above() })
	C.hc_move(C.int(x), C.int(y))
	C.hc_ctrl_fullwidth(C.int(MinW)) // 初始窗口宽 = MinW，控制条全宽（内层自动居中）
	C.xshape_mask(0) // 初始穿透
	st.mu.Lock()
	st.winW = MinW
	st.mu.Unlock()
	log.Printf("[widget] 桌面歌词挂件已启动 (pos=%d,%d)", x, y)
	return nil
}

func (d *gtkDisplay) Pos() (int, int) {
	var gx, gy C.int
	C.hc_get_pos(&gx, &gy)
	return int(gx), int(gy)
}

func (d *gtkDisplay) Move(x, y int) {
	C.hc_move(C.int(x), C.int(y))
}

func (d *gtkDisplay) Resize(w, h int) {
	C.hc_resize(C.int(w), C.int(h))
	C.gtk_widget_set_size_request(C.hc_draw(), C.gint(w), C.gint(h))
	// ctrl 全宽跟踪窗口宽 → 内层 center 自动居中按钮组（不手动算 cx）
	C.hc_ctrl_fullwidth(C.int(w))
}

func (d *gtkDisplay) Size() (int, int) {
	w := int(C.gtk_widget_get_width(C.hc_win()))
	h := int(C.gtk_widget_get_height(C.hc_win()))
	return w, h
}

func (d *gtkDisplay) ScreenSize() (int, int) {
	disp := C.gdk_display_get_default()
	monitors := C.gdk_display_get_monitors(disp)
	mon := C.g_list_model_get_item(monitors, 0)
	var geo C.GdkRectangle
	C.gdk_monitor_get_geometry((*C.GdkMonitor)(unsafe.Pointer(mon)), &geo)
	C.g_object_unref(C.gpointer(mon))
	return int(geo.width), int(geo.height)
}

func (d *gtkDisplay) CursorInside() bool {
	var px, py C.int
	if C.pointer_global(&px, &py) != 1 {
		return false
	}
	gx, gy := d.Pos()
	w, h := d.Size()
	return int(px) >= gx && int(px) <= gx+w && int(py) >= gy && int(py) <= gy+h
}

func (d *gtkDisplay) QueueDraw() {
	C.gtk_widget_queue_draw(C.hc_draw())
}

func (d *gtkDisplay) ShowUI(locked bool) {
	if locked {
		C.gtk_widget_set_visible(C.hc_ctrl(), 0)
		C.gtk_widget_set_visible(C.hc_lockbtn(), 1)
	} else {
		C.gtk_widget_set_visible(C.hc_ctrl(), 1)
		C.gtk_widget_set_visible(C.hc_lockbtn(), 0)
	}
	C.hc_bg_on() // 一体背景显示（浅色半透明）
	C.xshape_mask(1)
}

func (d *gtkDisplay) HideUI() {
	C.gtk_widget_set_visible(C.hc_ctrl(), 0)
	C.gtk_widget_set_visible(C.hc_lockbtn(), 0)
	C.hc_bg_off() // 背景移除 → 真透明（只剩歌词文字）
	C.xshape_mask(0)
}

// DrawCtrl Linux：GTK widget 已负责控制条绘制（cglue.c hc_ctrl），canvas no-op
func (d *gtkDisplay) DrawCtrl(shown, locked bool) {}

// HitTest Linux：GTK widget 已接管按钮事件（_hc_btn_cb），canvas 命中返回 0
func (d *gtkDisplay) HitTest(x, y float64) int { return 0 }

// SetVisible 整个挂件窗口显示/隐藏（下面面板 #d-tick 控制外部歌词）
func (d *gtkDisplay) SetVisible(on bool) {
	if on {
		C.hc_set_visible(1)
	} else {
		C.hc_set_visible(0)
	}
}

// SetPlayIcon 播放/暂停按钮图标（▶ ↔ ⏸）
func (d *gtkDisplay) SetPlayIcon(playing bool) {
	if playing {
		C.hc_set_play_icon(1)
	} else {
		C.hc_set_play_icon(0)
	}
}

// Tick Linux 由 cglue.c 的 g_timeout_add(150, goTick) 驱动，无需额外注册
func (d *gtkDisplay) Tick(delayMs int, f func()) {
	coreTick = f
}

// MainLoop GTK 主循环由宿主壳（wails）驱动；Windows/安卓版才需要独立消息循环
func (d *gtkDisplay) MainLoop() {}

func (d *gtkDisplay) Quit() {
	C.gtk_window_destroy((*C.GtkWindow)(unsafe.Pointer(C.hc_win())))
}

// ── cairo Canvas（平台绘制指令实装，照 GTK3 版） ──

type cairoCanvas struct{ cr *C.cairo_t }

func (c *cairoCanvas) Clear() {
	C.cairo_set_source_rgba(c.cr, 0, 0, 0, 0)
	C.cairo_paint(c.cr)
}

func (c *cairoCanvas) TextSize(text string, size float64) (float64, float64) {
	layout := C.pango_cairo_create_layout(c.cr)
	descStr := C.CString(fmt.Sprintf("Microsoft YaHei %d", int(size)))
	desc := C.pango_font_description_from_string(descStr)
	C.free(unsafe.Pointer(descStr)) // pango 拷字符串后需主动 free CString
	C.pango_layout_set_font_description(layout, desc)
	cs := C.CString(text)
	C.pango_layout_set_text(layout, cs, -1)
	var ink C.PangoRectangle
	C.pango_layout_get_pixel_extents(layout, &ink, nil)
	C.g_object_unref(C.gpointer(unsafe.Pointer(layout)))
	C.pango_font_description_free(desc)
	C.free(unsafe.Pointer(cs))
	return float64(ink.width), float64(ink.height)
}

func (c *cairoCanvas) Text(x, y float64, text string, size float64, r, g, b, a float64) {
	layout := C.pango_cairo_create_layout(c.cr)
	descStr := C.CString(fmt.Sprintf("Microsoft YaHei %d", int(size)))
	desc := C.pango_font_description_from_string(descStr)
	C.free(unsafe.Pointer(descStr)) // pango 拷字符串后需主动 free CString
	C.pango_layout_set_font_description(layout, desc)
	cs := C.CString(text)
	C.pango_layout_set_text(layout, cs, -1)
	C.cairo_set_source_rgba(c.cr, C.gdouble(r), C.gdouble(g), C.gdouble(b), C.gdouble(a))
	C.cairo_move_to(c.cr, C.gdouble(x), C.gdouble(y))
	C.pango_cairo_show_layout(c.cr, layout)
	C.g_object_unref(C.gpointer(unsafe.Pointer(layout)))
	C.pango_font_description_free(desc)
	C.free(unsafe.Pointer(cs))
}

// ── GTK 回调（cglue.c wire_signals 绑定）→ 核心逻辑 ──

//export goOnDraw
func goOnDraw(cr *C.cairo_t, width C.int, height C.int, user unsafe.Pointer) {
	paint(&cairoCanvas{cr: cr})
}

// ── 控制条按钮回调（cglue.c _hc_btn_cb 调用）→ 核心动作 ──

//export goOnPauseClick
func goOnPauseClick() {
	onPauseClick()
}

//export goOnMinusClick
func goOnMinusClick() {
	onMinusClick()
}

//export goOnPlusClick
func goOnPlusClick() {
	onPlusClick()
}

//export goOnLockClick
func goOnLockClick() {
	onLockClick()
}

//export goOnCloseClick
func goOnCloseClick() {
	onCloseClick()
}

//export goOnButtonPress
func goOnButtonPress(x, y float64, button int) {
	handlePress(x, y, button == 1)
}

//export goOnButtonRelease
func goOnButtonRelease() {
	handleRelease()
}

//export goOnMotion
func goOnMotion(x, y float64) {
	handleMotion(x, y)
}

//export goTick
func goTick(user unsafe.Pointer) C.gboolean {
	if coreTick != nil {
		coreTick()
	}
	return 1
}

