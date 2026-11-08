//go:build windows

// Windows 显示层：Win32 透明穿透悬浮窗
//   - 目标：WS_EX_LAYERED + LWA_COLORKEY 透明 + WS_EX_TRANSPARENT 鼠标穿透
//   - 注意：Wine（X11 后端）对 Layered+ColorKey 支持有限——窗口可能不可见；
//     WINEDEMO_VISIBLE=1 时降级为普通窗口（深色底），便于 Wine 下验证 GUI 本身
//   - GDI 绘制：白字黑阴影（与 Linux 版同视觉）；SetTimer 驱动核心 tick
//   - 控制条按钮（暂停/±/锁/关）暂缺——穿透态收不到鼠标事件，后续做
package widget

/*
#cgo windows LDFLAGS: -lgdi32 -luser32 -lkernel32
#include <windows.h>
#include <windowsx.h>
extern LRESULT CALLBACK goWndProc(HWND, UINT, WPARAM, LPARAM);
*/
import "C"

import (
	"fmt"
	"log"
	"os"
	"unsafe"
)

// 颜色键：背景刷这个颜色 → Layered Window 完全透明
const colorKeyR, colorKeyG, colorKeyB = 255, 0, 255

// visibleMode Wine/X11 下 Layered 透明窗不可见（兼容问题），降级普通窗口便于验证
var visibleMode = os.Getenv("WINEDEMO_VISIBLE") != ""

var hwndWin C.HWND

type winDisplay struct{}

func newDisplay() Display { return &winDisplay{} }

func (d *winDisplay) Init(x, y int) error {
	C.SetProcessDPIAware()
	inst := C.GetModuleHandle(nil)
	cn := C.CString("HcLyricWidgetClass")
	defer C.free(unsafe.Pointer(cn))

	cls := C.WNDCLASSEX{
		cbSize:        C.UINT(unsafe.Sizeof(C.WNDCLASSEX{})),
		style:         C.CS_HREDRAW | C.CS_VREDRAW,
		lpfnWndProc:   C.WNDPROC(unsafe.Pointer(C.goWndProc)),
		hInstance:     inst,
		hCursor:       C.LoadCursor(nil, C.IDC_ARROW),
		lpszClassName: cn,
	}
	if C.RegisterClassEx(&cls) == 0 && C.GetLastError() != C.ERROR_CLASS_ALREADY_EXISTS {
		return fmt.Errorf("RegisterClassEx 失败: %d", C.GetLastError())
	}

	title := C.CString("mymusic 桌面歌词")
	defer C.free(unsafe.Pointer(title))

	var exStyle C.DWORD = C.WS_EX_TOOLWINDOW | C.WS_EX_TOPMOST
	if !visibleMode {
		exStyle |= C.WS_EX_LAYERED // 透明（Wine X11 下不可见，真 Windows 正常）
		// WS_EX_TRANSPARENT 鼠标穿透：Wine/真机都支持；可见模式保留（便于演示交互）
		exStyle |= C.WS_EX_TRANSPARENT
	}
	hwnd := C.CreateWindowEx(
		exStyle,
		cn, title, C.WS_POPUP,
		C.int(x), C.int(y), C.int(MinW), C.int(WinH),
		nil, nil, inst, nil,
	)
	if hwnd == nil {
		return fmt.Errorf("CreateWindowEx 失败: %d", C.GetLastError())
	}
	hwndWin = hwnd
	if !visibleMode {
		// ColorKey 透明（RGB 宏 cgo 不认，手动位运算）
		C.SetLayeredWindowAttributes(hwnd, colorRef(colorKeyR, colorKeyG, colorKeyB), 0, C.LWA_COLORKEY)
	}
	C.ShowWindow(hwnd, C.SW_SHOWNOACTIVATE)
	st.mu.Lock()
	st.winW = MinW
	st.mu.Unlock()
	log.Printf("[widget] Windows 桌面歌词挂件已启动 (pos=%d,%d) visible=%v", x, y, visibleMode)
	return nil
}

func (d *winDisplay) Pos() (int, int) {
	var rc C.RECT
	C.GetWindowRect(hwndWin, &rc)
	return int(rc.left), int(rc.top)
}

func (d *winDisplay) Move(x, y int) {
	C.SetWindowPos(hwndWin, C.HWND(nil), C.int(x), C.int(y), 0, 0, C.SWP_NOSIZE|C.SWP_NOZORDER|C.SWP_NOACTIVATE)
}

func (d *winDisplay) Resize(w, h int) {
	C.SetWindowPos(hwndWin, C.HWND(nil), 0, 0, C.int(w), C.int(h), C.SWP_NOMOVE|C.SWP_NOZORDER|C.SWP_NOACTIVATE)
}

func (d *winDisplay) Size() (int, int) {
	var rc C.RECT
	C.GetClientRect(hwndWin, &rc)
	return int(rc.right), int(rc.bottom)
}

func (d *winDisplay) ScreenSize() (int, int) {
	return int(C.GetSystemMetrics(C.SM_CXSCREEN)), int(C.GetSystemMetrics(C.SM_CYSCREEN))
}

func (d *winDisplay) CursorInside() bool {
	var pt C.POINT
	if C.GetCursorPos(&pt) == 0 {
		return false
	}
	var rc C.RECT
	C.GetWindowRect(hwndWin, &rc)
	return int(pt.x) >= int(rc.left) && int(pt.x) <= int(rc.right) &&
		int(pt.y) >= int(rc.top) && int(pt.y) <= int(rc.bottom)
}

func (d *winDisplay) QueueDraw() {
	C.InvalidateRect(hwndWin, nil, 0) // FALSE：不擦背景，防闪烁
}

// 穿透切换：ShowUI 取消 WS_EX_TRANSPARENT（接收鼠标，控制条可点/可拖）；HideUI 恢复穿透
func setClickable(on bool) {
	if hwndWin == nil {
		return
	}
	ex := C.GetWindowLongPtr(hwndWin, C.GWL_EXSTYLE)
	if on {
		ex &^= C.WS_EX_TRANSPARENT
	} else {
		ex |= C.WS_EX_TRANSPARENT
	}
	C.SetWindowLongPtr(hwndWin, C.GWL_EXSTYLE, ex)
}

// ShowUI 悬停浮现控制条（locked=true 只出小锁）：取消穿透接收鼠标，重绘画控制条
func (d *winDisplay) ShowUI(locked bool) {
	setClickable(true)
	C.InvalidateRect(hwndWin, nil, 0)
}

// HideUI 收起控制条：恢复穿透（鼠标穿透，只显示歌词）
func (d *winDisplay) HideUI() {
	setClickable(false)
	C.InvalidateRect(hwndWin, nil, 0)
}
// SetVisible Windows 版（TODO 挂件窗口显示/隐藏）
func (d *winDisplay) SetVisible(on bool) {}

func (d *winDisplay) Tick(delayMs int, f func()) {
	coreTick = f
	C.SetTimer(hwndWin, 1, C.UINT(delayMs), nil)
}

// MainLoop 消息循环（阻塞）
func (d *winDisplay) MainLoop() {
	var msg C.MSG
	for C.GetMessage(&msg, nil, 0, 0) > 0 {
		C.TranslateMessage(&msg)
		C.DispatchMessage(&msg)
	}
}

func (d *winDisplay) Quit() {
	C.PostMessage(hwndWin, C.WM_CLOSE, 0, 0)
}

// ── GDI Canvas（绘制指令实装） ──

type winCanvas struct{ dc C.HDC }

func (c *winCanvas) Clear() {
	var rc C.RECT
	C.GetClientRect(hwndWin, &rc)
	var br C.HBRUSH
	if visibleMode {
		br = C.CreateSolidBrush(colorRef(17, 17, 27)) // 可见模式：深色底（Wine 降级验证）
	} else {
		br = C.CreateSolidBrush(colorRef(colorKeyR, colorKeyG, colorKeyB)) // 透明模式：洋红 ColorKey
	}
	C.FillRect(c.dc, &rc, br)
	C.DeleteObject(C.HGDIOBJ(unsafe.Pointer(br)))
}

func createFont(size float64) C.HFONT {
	return C.CreateFont(
		C.int(-size), 0, 0, 0, C.FW_NORMAL, 0, 0, 0,
		C.DEFAULT_CHARSET, C.OUT_DEFAULT_PRECIS, C.CLIP_DEFAULT_PRECIS,
		C.CLEARTYPE_QUALITY, C.DEFAULT_PITCH|C.FF_DONTCARE,
		C.CString("Microsoft YaHei"),
	)
}

func (c *winCanvas) TextSize(text string, size float64) (float64, float64) {
	f := createFont(size)
	old := C.SelectObject(c.dc, C.HGDIOBJ(unsafe.Pointer(f)))
	var tm C.TEXTMETRIC
	C.GetTextMetrics(c.dc, &tm)
	cs := C.CString(text)
	var sz C.SIZE
	C.GetTextExtentPoint32(c.dc, cs, C.int(len(text)), &sz)
	C.free(unsafe.Pointer(cs))
	C.SelectObject(c.dc, old)
	C.DeleteObject(C.HGDIOBJ(unsafe.Pointer(f)))
	return float64(sz.cx), float64(tm.tmHeight)
}

func (c *winCanvas) Text(x, y float64, text string, size float64, r, g, b, a float64) {
	f := createFont(size)
	old := C.SelectObject(c.dc, C.HGDIOBJ(unsafe.Pointer(f)))
	C.SetTextColor(c.dc, colorRef(int(r*255), int(g*255), int(b*255)))
	C.SetBkMode(c.dc, C.TRANSPARENT)
	cs := C.CString(text)
	C.TextOut(c.dc, C.int(x), C.int(y), cs, C.int(len(text)))
	C.free(unsafe.Pointer(cs))
	C.SelectObject(c.dc, old)
	C.DeleteObject(C.HGDIOBJ(unsafe.Pointer(f)))
}

// ── 控制条（GDI 手绘，同 Linux 视觉/顺序/位置：⏸−+🔓✕ 底部居中） ──

// ctrlRects 计算 5 按钮矩形（窗口内坐标，底部居中；paint 绘制与 HitTest 共用）
// 用共享按钮表 ctrlBtnTable（顺序/尺寸/kind 一份，只差异 GDI 布局）
func ctrlRects() [5]struct{ x, y, w, h int } {
	var out [5]struct{ x, y, w, h int }
	rc := C.RECT{}
	C.GetClientRect(hwndWin, &rc)
	winW := int(rc.right)
	btns := ctrlBtnTable()
	// 总宽 = 5 按钮宽 + 4 间隙(6)
	total := 0
	for _, b := range btns {
		total += b.W
	}
	total += 4 * 6
	x0 := (winW - total) / 2
	y0 := WinH - CtrlH
	cx := x0
	for i, b := range btns {
		out[i] = struct{ x, y, w, h int }{cx, y0, b.W, CtrlH}
		cx += b.W + 6
	}
	return out
}

// DrawCtrl 绘制控制条按钮（GDI 文字，透明底白字；同 Linux #hc-btn 视觉）
func (d *winDisplay) DrawCtrl(shown, locked bool) {
	if !shown {
		return
	}
	if locked {
		// 锁定态：只出小锁按钮（对齐 Linux hc_lockbtn 语义）
		drawCtrlButton(4)
		return
	}
	for k := 1; k <= 5; k++ {
		drawCtrlButton(k)
	}
}

// drawCtrlButton 画单个按钮（GDI 文字；hover 反馈由绘制层简化——透明底白字）
func drawCtrlButton(kind int) {
	btns := ctrlBtnTable()
	rects := ctrlRects()
	if kind < 1 || kind > len(btns) {
		return
	}
	r := rects[kind-1]
	// 用共享 paint 的 Canvas 画文字（GDI TextOut），颜色 #e8e8f4
	dc := C.GetDC(hwndWin)
	if dc == nil {
		return
	}
	c := &winCanvas{dc: dc}
	// 文字垂直居中（CtrlH=24，字体 16）
	c.Text(float64(r.x+4), float64(r.y+4), btns[kind-1].Label, 16, 0.91, 0.91, 0.96, 1)
	C.ReleaseDC(hwndWin, dc)
}

// HitTest 按钮命中（handlePress 传屏幕坐标 → 转窗口内坐标 → 命中 kind；0=未命中）
func (d *winDisplay) HitTest(x, y float64) int {
	gx, gy := d.Pos()
	lx, ly := int(x)-gx, int(y)-gy
	rects := ctrlRects()
	for i := 0; i < 5; i++ {
		r := rects[i]
		if lx >= r.x && lx <= r.x+r.w && ly >= r.y && ly <= r.y+r.h {
			return i + 1
		}
	}
	return 0
}

// ── 窗口过程（//export，单窗口） ──

//export goWndProc
func goWndProc(h C.HWND, msg C.UINT, wp C.WPARAM, lp C.LPARAM) C.LRESULT {
	switch msg {
	case C.WM_PAINT:
		var ps C.PAINTSTRUCT
		dc := C.BeginPaint(h, &ps)
		paint(&winCanvas{dc: dc})
		C.EndPaint(h, &ps)
		return 0
	case C.WM_TIMER:
		if wp == 1 && coreTick != nil {
			coreTick()
		}
		return 0
	case C.WM_LBUTTONDOWN:
		sx, sy := clientToScreen(lp)
		handlePress(float64(sx), float64(sy), true)
		return 0
	case C.WM_LBUTTONUP:
		handleRelease()
		return 0
	case C.WM_MOUSEMOVE:
		sx, sy := clientToScreen(lp)
		handleMotion(float64(sx), float64(sy))
		return 0
	case C.WM_ERASEBKGND:
		return 1 // 不擦背景（自绘防闪）
	case C.WM_DESTROY:
		C.PostQuitMessage(0)
		return 0
	}
	return C.DefWindowProc(h, msg, wp, lp)
}

func clientToScreen(lp C.LPARAM) (int, int) {
	l := uint64(lp)
	var pt C.POINT
	pt.x = C.LONG(int16(l & 0xFFFF))          // LOWORD（有符号）
	pt.y = C.LONG(int16((l >> 16) & 0xFFFF)) // HIWORD
	C.ClientToScreen(hwndWin, &pt)
	return int(pt.x), int(pt.y)
}

// colorRef 拼 COLORREF（RGB 宏 cgo 不认，手动位运算）
func colorRef(r, g, b int) C.COLORREF {
	return C.COLORREF(C.DWORD(uint32(r&0xFF) | uint32(g&0xFF)<<8 | uint32(b&0xFF)<<16))
}
