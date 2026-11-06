// Package widget 桌面歌词挂件——多态显示层
//
// 核心状态机/歌词拉取/交互逻辑平台无关（widget.go），显示层按平台实现：
//   - linux:   display_linux.go   （cgo + GTK4 + X11 穿透，GTK3 在 wails v3.1.0 下启动即崩）
//   - windows: display_windows.go （Win32 透明穿透窗口，待真 Windows 环境补）
//   - android: 悬浮窗 Service（wails3 壳 Java 层，未开发；widget_android.go 当前 no-op）
package widget

// Canvas 平台无关绘制指令（linux=cairo/pango，windows=GDI 待实现）
type Canvas interface {
	Clear()                                  // 清屏为全透明
	TextSize(text string, size float64) (w, h float64) // 文本度量（像素，含墨水高度）
	Text(x, y float64, text string, size float64, r, g, b, a float64) // 左上角基点为 (x,y) 绘制文本
}

// Display 桌面歌词显示层接口
type Display interface {
	// Init 创建无边框置顶穿透窗口并置于 (x,y)；返回错误则放弃挂件
	Init(x, y int) error
	// Pos/Move 窗口位置（屏幕坐标）
	Pos() (int, int)
	Move(x, y int)
	// Resize 调整窗口内容区尺寸
	Resize(w, h int)
	// Size 当前窗口内容区尺寸
	Size() (int, int)
	// ScreenSize 屏幕尺寸（默认位置居中用）
	ScreenSize() (int, int)
	// CursorInside 指针是否在窗口内（悬停检测）
	CursorInside() bool
	// QueueDraw 请求下一帧重绘（核心循环推进动画）
	QueueDraw()
	// ShowUI 悬停浮现控制条（locked=true 只出小锁）；HideUI 收起
	ShowUI(locked bool)
	HideUI()
	// SetVisible 整个挂件窗口显示/隐藏（下面面板 #d-tick → /api/v1/widget/lyric-visible）
	SetVisible(on bool)
	// Tick 周期调度核心循环（delayMs 毫秒后执行一次并自动续期）
	Tick(delayMs int, f func())
	// MainLoop 阻塞事件循环；Quit 退出
	MainLoop()
	Quit()
}
