//go:build linux

// Linux 显示层：cgo + GTK3 + X11 真穿透（从原 widget.go 迁出，逻辑保留）
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
	"fmt"
	"log"
	"unsafe"
)

// gtkDisplay Linux GTK3 显示层实现
type gtkDisplay struct{}

func newDisplay() Display { return &gtkDisplay{} }

func (d *gtkDisplay) Init(x, y int) error {
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
	mkBtn("‖", 24, 1)    // 暂停/播放
	mkBtn("−0.5", 44, 2) // 微调
	mkBtn("+0.5", 44, 3)
	mkBtn("🔓", 28, 4) // 锁定切换
	mkBtn("×", 22, 5)  // 关闭
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

func (d *gtkDisplay) Pos() (int, int) {
	var gx, gy C.gint
	C.gtk_window_get_position((*C.GtkWindow)(unsafe.Pointer(C.hc_win())), &gx, &gy)
	return int(gx), int(gy)
}

func (d *gtkDisplay) Move(x, y int) {
	C.gtk_window_move((*C.GtkWindow)(unsafe.Pointer(C.hc_win())), C.gint(x), C.gint(y))
}

func (d *gtkDisplay) Resize(w, h int) {
	C.gtk_window_resize((*C.GtkWindow)(unsafe.Pointer(C.hc_win())), C.gint(w), C.gint(h))
}

func (d *gtkDisplay) Size() (int, int) {
	w := int(C.gtk_widget_get_allocated_width(C.hc_win()))
	h := int(C.gtk_widget_get_allocated_height(C.hc_win()))
	return w, h
}

func (d *gtkDisplay) ScreenSize() (int, int) {
	return int(C.gdk_screen_width()), int(C.gdk_screen_height())
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
	C.gtk_widget_queue_draw(C.hc_win())
}

func (d *gtkDisplay) ShowUI(locked bool) {
	if locked {
		C.gtk_widget_hide(C.hc_ctrl())
		C.gtk_widget_show(C.hc_lockbtn())
	} else {
		C.gtk_widget_show(C.hc_ctrl())
		C.gtk_widget_hide(C.hc_lockbtn())
	}
	C.xshape_mask(1)
}

func (d *gtkDisplay) HideUI() {
	C.gtk_widget_hide(C.hc_ctrl())
	C.gtk_widget_hide(C.hc_lockbtn())
	C.xshape_mask(0)
}

// Tick Linux 由 cglue.c 的 g_timeout_add(150, goTick) 驱动，无需额外注册
func (d *gtkDisplay) Tick(delayMs int, f func()) {
	coreTick = f
}

// MainLoop GTK 主循环由宿主壳（wails）驱动；Windows 版才需要独立消息循环
func (d *gtkDisplay) MainLoop() {}

func (d *gtkDisplay) Quit() {
	C.gtk_widget_destroy(C.hc_win())
}

// ── cairo Canvas（平台绘制指令实装） ──

type cairoCanvas struct{ cr *C.cairo_t }

func (c *cairoCanvas) Clear() {
	C.cairo_set_source_rgba(c.cr, 0, 0, 0, 0)
	C.cairo_paint(c.cr)
}

func (c *cairoCanvas) TextSize(text string, size float64) (float64, float64) {
	layout := C.pango_cairo_create_layout(c.cr)
	desc := C.pango_font_description_from_string(C.CString(fmt.Sprintf("Microsoft YaHei %d", int(size))))
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
	desc := C.pango_font_description_from_string(C.CString(fmt.Sprintf("Microsoft YaHei %d", int(size))))
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
func goOnDraw(w *C.GtkWidget, cr *C.cairo_t, user unsafe.Pointer) C.gboolean {
	paint(&cairoCanvas{cr: cr})
	return 1
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
func goOnButtonPress(w *C.GtkWidget, e *C.GdkEventButton, u unsafe.Pointer) C.gboolean {
	handlePress(float64(e.x_root), float64(e.y_root), e.button == 1)
	return 1
}

//export goOnButtonRelease
func goOnButtonRelease(w *C.GtkWidget, e *C.GdkEventButton, u unsafe.Pointer) C.gboolean {
	handleRelease()
	return 1
}

//export goOnMotion
func goOnMotion(w *C.GtkWidget, e *C.GdkEventMotion, u unsafe.Pointer) C.gboolean {
	handleMotion(float64(e.x_root), float64(e.y_root))
	return 1
}

//export goTick
func goTick(user unsafe.Pointer) C.gboolean {
	if coreTick != nil {
		coreTick()
	}
	return 1
}
