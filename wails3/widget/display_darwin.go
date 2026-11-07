//go:build darwin

// macOS 显示层：Cocoa/AppKit（NSWindow 无边框置顶透明 + ignoresMouseEvents 穿透）。
// 照 Linux(cglue.c)/Windows(GDI) 模式：ObjC 核心 darwin_display.m → Go 回调。
// 视觉/行为一致：悬停浮现控制条（⏸−+🔓✕ 底部居中，共享表 ctrlBtnTable）、
// 锁定态只出小锁、拖拽、关闭。
package widget

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework QuartzCore
#include <stdlib.h>
#include <stdint.h>
// darwin_display.m 提供
extern int hc_mac_init(int x, int y, int w, int h, double scale);
extern void hc_mac_clickable(int on);
extern int hc_mac_pos(int *px, int *py);
extern void hc_mac_move(int x, int y);
extern void hc_mac_resize(int w, int h);
extern int hc_mac_size(int *w, int *h);
extern int hc_mac_screen(int *w, int *h);
extern void hc_mac_redraw(void);
extern void hc_mac_visible(int on);
extern void hc_mac_tick(double ms);
extern void hc_mac_run(void);
extern int hc_mac_mouse(int *px, int *py);
extern void hc_mac_clear(void);
extern void hc_mac_text_size(const char *text, double size, double *w, double *h);
extern void hc_mac_text(double x, double y, const char *text, double size, double r, double g, double b, double a);
extern void hc_mac_fill_rect(double x, double y, double w, double h, double r, double g, double b, double a);
extern void hc_mac_quit(void);
*/
import "C"

import (
	"log"
	"unsafe"
)

// 屏幕缩放（Retina 1x/2x；坐标用逻辑点，同 Linux/Windows 像素对齐）
var macScale = 1.0

type macDisplay struct{}

func newDisplay() Display { return &macDisplay{} }

func (d *macDisplay) Init(x, y int) error {
	// macOS Y 轴向上（底部原点），widget.go 传顶部坐标 y——翻转成底部坐标
	_, sh := d.ScreenSize()
	yFlip := sh - y - WinH // 顶部坐标 → 底部坐标
	if C.hc_mac_init(C.int(x), C.int(yFlip), C.int(MinW), C.int(WinH), C.double(macScale)) == 0 {
		return nil // 幂等：已初始化
	}
	log.Printf("[widget] macOS 桌面歌词挂件已启动 (pos=%d,%d→flip=%d)", x, y, yFlip)
	return nil
}

func (d *macDisplay) Pos() (int, int) {
	var px, py C.int
	C.hc_mac_pos(&px, &py)
	return int(px), int(py)
}

func (d *macDisplay) Move(x, y int) { C.hc_mac_move(C.int(x), C.int(y)) }

func (d *macDisplay) Resize(w, h int) { C.hc_mac_resize(C.int(w), C.int(h)) }

func (d *macDisplay) Size() (int, int) {
	var w, h C.int
	C.hc_mac_size(&w, &h)
	return int(w), int(h)
}

func (d *macDisplay) ScreenSize() (int, int) {
	var w, h C.int
	C.hc_mac_screen(&w, &h)
	return int(w), int(h)
}

func (d *macDisplay) CursorInside() bool {
	var px, py C.int
	if C.hc_mac_mouse(&px, &py) != 1 {
		log.Printf("[mac-debug] CursorInside: mouse fail")
		return false
	}
	gx, gy := d.Pos()
	w, h := d.Size()
	inside := int(px) >= gx && int(px) <= gx+w && int(py) >= gy && int(py) <= gy+h
	log.Printf("[mac-debug] CursorInside mouse=(%d,%d) win=(%d,%d,%d,%d) inside=%v",
		int(px), int(py), gx, gy, w, h, inside)
	return inside
}

func (d *macDisplay) QueueDraw() { C.hc_mac_redraw() }

func (d *macDisplay) ShowUI(locked bool) {
	log.Printf("[mac-debug] ShowUI locked=%v", locked)
	C.hc_mac_clickable(1)
	C.hc_mac_redraw()
}

func (d *macDisplay) HideUI() {
	log.Printf("[mac-debug] HideUI")
	C.hc_mac_clickable(0)
	C.hc_mac_redraw()
}

// DrawCtrl macOS：控制条 + 聚焦框（同 Windows GDI 语义——按钮文字/聚焦框走 Canvas）
func (d *macDisplay) DrawCtrl(shown, locked bool) {
	if !shown {
		return
	}
	c := &macCanvas{}
	w, _ := d.Size()
	// 聚焦框（showUI 出框）：四周细框 #e8e8f4 alpha 0.85
	br, bg, bb, ba := 0.91, 0.91, 0.96, 0.85
	c.FillRect(0, 0, float64(w), 1.2, br, bg, bb, ba)     // 上
	c.FillRect(0, float64(WinH)-1.2, float64(w), 1.2, br, bg, bb, ba) // 下
	c.FillRect(0, 0, 1.2, float64(WinH), br, bg, bb, ba)   // 左
	c.FillRect(float64(w)-1.2, 0, 1.2, float64(WinH), br, bg, bb, ba) // 右
	if locked {
		// 锁定态：只出小锁按钮
		drawMacCtrlButton(4)
		return
	}
	for k := 1; k <= 5; k++ {
		drawMacCtrlButton(k)
	}
}

// drawMacCtrlButton 画单个按钮文字（Canvas.Text，同 Windows drawCtrlButton）
func drawMacCtrlButton(kind int) {
	btns := ctrlBtnTable()
	rects := macCtrlRects()
	if kind < 1 || kind > len(btns) {
		return
	}
	r := rects[kind-1]
	c := &macCanvas{}
	c.Text(float64(r.x+4), float64(r.y+4), btns[kind-1].Label, 16, 0.91, 0.91, 0.96, 1)
}

// HitTest macOS：控制条命中（屏幕坐标 → 窗口内 → 共享表矩形，同 Windows ctrlRects）
func (d *macDisplay) HitTest(x, y float64) int {
	gx, gy := d.Pos()
	lx, ly := int(x)-gx, int(y)-gy
	rects := macCtrlRects()
	btns := ctrlBtnTable()
	for i, b := range btns {
		r := rects[i]
		if lx >= r.x && lx <= r.x+r.w && ly >= r.y && ly <= r.y+r.h {
			return b.Kind
		}
	}
	return 0
}

func (d *macDisplay) SetVisible(on bool) { C.hc_mac_visible(C.int(bool2int(on))) }

func (d *macDisplay) Tick(delayMs int, f func()) {
	log.Printf("[mac-debug] Tick(%dms)", delayMs)
	goTickFn = f
	C.hc_mac_tick(C.double(delayMs))
}

func (d *macDisplay) MainLoop() {
	// [NSApp run] 必须主线程执行；hc_mac_run 内部用 dispatch_async 调度
	// 这里调用后返回（非阻塞），NSApp 主循环在后台线程驱动
	C.hc_mac_run()
}

func (d *macDisplay) Quit() { C.hc_mac_quit() }

// ── Go 回调（darwin_display.m → //export） ──

//export goOnDraw
func goOnDraw(width, height C.int) {
	if disp != nil {
		disp.QueueDraw()
		// 绘制由共享 paint 走 Canvas（darwinCanvas 用当前 NSGraphicsContext）
		paint(&macCanvas{})
	}
}

//export goOnPress
func goOnPress(x, y C.double, button C.int) {
	log.Printf("[mac-debug] goOnPress (%.0f,%.0f) btn=%d", x, y, button)
	handlePress(float64(x), float64(y), button == 0)
}

//export goOnRelease
func goOnRelease() { handleRelease() }

//export goOnMotion
func goOnMotion(x, y C.double) { handleMotion(float64(x), float64(y)) }

// ── macCanvas：Canvas 实装（ObjC 绘制，drawRect 内 NSGraphicsContext 当前有效） ──

type macCanvas struct{}

func (c *macCanvas) Clear() { C.hc_mac_clear() }

func (c *macCanvas) TextSize(text string, size float64) (float64, float64) {
	cs := C.CString(text)
	defer C.free(unsafe.Pointer(cs))
	var w, h C.double
	C.hc_mac_text_size(cs, C.double(size), &w, &h)
	return float64(w), float64(h)
}

func (c *macCanvas) Text(x, y float64, text string, size float64, r, g, b, a float64) {
	cs := C.CString(text)
	defer C.free(unsafe.Pointer(cs))
	C.hc_mac_text(C.double(x), C.double(y), cs, C.double(size), C.double(r), C.double(g), C.double(b), C.double(a))
}
func (c *macCanvas) FillRect(x, y, w, h float64, r, g, b, a float64) {
	C.hc_mac_fill_rect(C.double(x), C.double(y), C.double(w), C.double(h), C.double(r), C.double(g), C.double(b), C.double(a))
}

// ── macCtrlRects：控制条按钮矩形（窗口内，底部居中；同 Windows ctrlRects） ──

func macCtrlRects() [5]struct{ x, y, w, h int } {
	var out [5]struct{ x, y, w, h int }
	w, _ := (&macDisplay{}).Size()
	btns := ctrlBtnTable()
	total := 0
	for _, b := range btns {
		total += b.W
	}
	total += 4 * 6
	x0 := (w - total) / 2
	y0 := WinH - CtrlH
	cx := x0
	for i, b := range btns {
		out[i] = struct{ x, y, w, h int }{cx, y0, b.W, CtrlH}
		cx += b.W + 6
	}
	return out
}

func bool2int(b bool) int {
	if b {
		return 1
	}
	return 0
}

// goTickFn 存 Go 回调（hc_mac_tick fn 桥）
var goTickFn func()

//export _hc_tick_bridge
func _hc_tick_bridge() {
	if goTickFn != nil {
		goTickFn()
	}
}
