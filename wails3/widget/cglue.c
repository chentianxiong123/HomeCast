//go:build linux

// cglue.c — GTK4 挂件 C 实现（照 GTK3 版逻辑：draw_func + 手势 + tick + 1bpp 穿透）
// Linux 专属（GTK4 + X11）；Windows 用 display_windows.go（GDI，不编本文件）
#include <gtk/gtk.h>
#include <gdk/x11/gdkx.h>
#include <X11/Xlib.h>
#include <X11/Xatom.h>
#include <X11/extensions/shape.h>

// Go 侧回调（display_linux.go //export）
extern void goOnDraw(cairo_t *cr, int width, int height, gpointer user);
extern void goOnButtonPress(double x, double y, int button);
extern void goOnButtonRelease(void);
extern void goOnMotion(double x, double y);
extern gboolean goTick(gpointer user);
extern void goOnPauseClick(void);
extern void goOnMinusClick(void);
extern void goOnPlusClick(void);
extern void goOnLockClick(void);
extern void goOnCloseClick(void);
extern void goInit(void);
extern int pointer_global(int *x, int *y);

// X11 底层前向声明（定义在下方）
void hc_get_pos(int *x, int *y);
static gboolean _map_cb(GtkWidget *w, GdkEvent *ev, gpointer u);

// Xlib 多线程安全（review 跨线程 #7）：time.AfterFunc 回调（非 GTK 主线程）会调
// hc_keep_above/hc_move 等 Xlib 调用，共享 GDK 的 Display 连接。必须在进程首次任何
// Xlib 调用之前初始化 XInitThreads（开启 Xlib 内部多线程锁），否则并发 Xlib 调用
// 数据竞争/崩溃。构造函数在库加载即执行，早于 goInit/hc_move。
__attribute__((constructor)) static void _hc_xlib_threads_init(void) {
	XInitThreads();
}

static GtkWidget *g_win = NULL;
static GtkWidget *g_fixed = NULL;
static GtkWidget *g_ctrl = NULL;
static GtkWidget *g_lockbtn = NULL;
static GtkWidget *g_draw = NULL;
static GtkWidget *g_playbtn = NULL; // 播放/暂停按钮（图标随状态切换）

GtkWidget* hc_win(void)     { return g_win; }
GtkWidget* hc_ctrl(void)    { return g_ctrl; }
GtkWidget* hc_lockbtn(void) { return g_lockbtn; }
GtkWidget* hc_playbtn(void) { return g_playbtn; }

void hc_set_playbtn(GtkWidget *w) { g_playbtn = w; }

// 播放/暂停按钮图标切换：playing=1 ⏸（暂停符号），0 ▶（播放符号）
void hc_set_play_icon(int playing) {
	if (!g_playbtn) return;
	gtk_button_set_label(GTK_BUTTON(g_playbtn), playing ? "⏸" : "▶");
}

// 桌面挂件整个窗口显示/隐藏（下面面板 #d-tick → POST /api/v1/widget/lyric-visible）
void hc_set_visible(int on) {
    if (!g_win) return;
    if (on) gtk_widget_set_visible(g_win, TRUE);
    else    gtk_widget_set_visible(g_win, FALSE);
}
GtkWidget* hc_fixed(void)   { return g_fixed; }
GtkWidget* hc_draw(void)    { return g_draw; }
void hc_set_win(GtkWidget *w)     { g_win = w; }
void hc_set_ctrl(GtkWidget *w)    { g_ctrl = w; }
void hc_set_lockbtn(GtkWidget *w) { g_lockbtn = w; }
void hc_set_fixed(GtkWidget *w)   { g_fixed = w; }
void hc_set_draw_area(GtkWidget *w) { g_draw = w; }

// GTK4 严格主线程：Init 经 g_idle_add 调度到主循环
static gboolean _init_idle(gpointer user) { (void)user; goInit(); return G_SOURCE_REMOVE; }
void _init_cb(void) { g_idle_add(_init_idle, NULL); }

// ── draw_func → goOnDraw ──
static void _draw_cb(GtkDrawingArea *da, cairo_t *cr, int width, int height, gpointer user) {
	(void)da; (void)user;
	goOnDraw(cr, width, height, user);
}

// ── 按钮回调 → Go 动作（照 GTK3 版） ──
static void _hc_btn_cb(GtkWidget *b, gpointer d) {
	(void)b;
	switch (GPOINTER_TO_INT(d)) {
	case 1: goOnPauseClick(); break;
	case 2: goOnMinusClick(); break;
	case 3: goOnPlusClick(); break;
	case 4: goOnLockClick(); break;
	case 5: goOnCloseClick(); break;
	}
}

void _hc_btn_connect(GtkWidget *b, int kind) {
	g_signal_connect(b, "clicked", G_CALLBACK(_hc_btn_cb), GINT_TO_POINTER(kind));
}

// ── 按下/释放/拖动：全局坐标（X11 XQueryPointer，和 CursorInside 同源）──
static void _press_cb(GtkGestureClick *g, int n, double x, double y, gpointer u) {
	(void)n; (void)u;
	int px, py;
	if (!pointer_global(&px, &py)) {
		int gx, gy; hc_get_pos(&gx, &gy); px = (int)x + gx; py = (int)y + gy;
	}
	int btn = gtk_gesture_single_get_current_button(GTK_GESTURE_SINGLE(g));
	goOnButtonPress(px, py, btn);
}

static void _release_cb(GtkGestureClick *g, int n, double x, double y, gpointer u) {
	(void)g; (void)n; (void)x; (void)y; (void)u;
	goOnButtonRelease();
}

// ── 自由拖动（GtkGestureDrag，双向；拖动用全局指针坐标）──
static void _drag_cb(GtkGestureDrag *g, double x, double y, gpointer u) {
	(void)g; (void)x; (void)y; (void)u;
	int px, py;
	if (pointer_global(&px, &py)) {
		goOnMotion(px, py);
	}
}

// ── CSS 样式（字幕 + 面板一体：窗口整面浅色半透明底，hover 时才显示）──
void _hc_css(void) {
	GtkCssProvider *p = gtk_css_provider_new();
	const char *css =
		"window, window.background { background-color: transparent; }"
		"drawingarea { background-color: transparent; }"
		// 一体背景：悬浮（.hc-hover）时**保持透明**，只加 1px 描边（ring）标识悬浮态
		"window.hc-hover {"
		" background-color: transparent;"
		" border-radius: 14px;"
		" box-shadow: 0 0 0 1px rgba(255,255,255,110); }"  // 悬浮描边，不填充
		"window.hc-hover drawingarea { background-color: transparent; }"
		// 控制条：面板融入一体背景（透明底，无卡片）
		"#hc-ctrl { background: transparent; background-color: transparent;"
		" border: none; box-shadow: none; padding: 2px 10px; margin: 0px; }"
		// 按钮：透明底、大字号（16px）、白字（透明背景上可读）
		"#hc-btn { background: transparent; color: #e8e8f4;"
		" border: none; border-radius: 8px; font-size: 16px; padding: 3px 8px;"
		" min-height: 18px; min-width: 18px; box-shadow: none; outline: none; margin: 0px;"
		" font-family: 'Noto Sans', 'Cantarell', 'DejaVu Sans', sans-serif; }"
		"#hc-btn label { margin: 0px; padding: 0px; color: #e8e8f4; font-size: 16px;"
		" font-family: 'Noto Sans', 'Cantarell', 'DejaVu Sans', sans-serif; }"
		"#hc-btn:hover { background: rgba(255,255,255,35); }"
		"#hc-btn:hover label { color: #ffffff; }"
		// 锁定态小锁：半透明深底 + 白字 + 浅描边（常显控件需可读）
		"#hc-lock { background: rgba(16,16,24,120); color: #e8e8f4;"
		" border: 1px solid rgba(255,255,255,40); border-radius: 8px; font-size: 13px; padding: 2px 5px;"
		" box-shadow: 0 1px 5px rgba(0,0,0,80); }"
		"#hc-lock label { color: #e8e8f4; font-size: 13px; margin: 0px; padding: 0px; }"
		"#hc-lock:hover { background: rgba(255,255,255,35); }";
	gtk_css_provider_load_from_string(p, css);
	GdkDisplay *disp = gdk_display_get_default();
	if (disp == NULL) disp = gtk_widget_get_display(g_win);
	gtk_style_context_add_provider_for_display(
		disp, GTK_STYLE_PROVIDER(p),
		GTK_STYLE_PROVIDER_PRIORITY_APPLICATION);
	g_object_unref(p);
}

// ── 一体背景显隐：hover 时给窗口加 .hc-hover 类（整面浅色半透明），离开移除（真透明）──
void hc_bg_on(void) {
	if (!g_win) return;
	gtk_widget_add_css_class(g_win, "hc-hover");
}
void hc_bg_off(void) {
	if (!g_win) return;
	gtk_widget_remove_css_class(g_win, "hc-hover");
}

// ── wire_signals：draw_func + 手势 + tick（照 GTK3 版 g_timeout_add）──
void wire_signals(void) {
	gtk_drawing_area_set_draw_func(GTK_DRAWING_AREA(g_draw), _draw_cb, NULL, NULL);

	GtkGesture *click = gtk_gesture_click_new();
	gtk_gesture_single_set_button(GTK_GESTURE_SINGLE(click), 0);
	g_signal_connect(click, "pressed", G_CALLBACK(_press_cb), NULL);
	g_signal_connect(click, "released", G_CALLBACK(_release_cb), NULL);
	gtk_widget_add_controller(g_draw, GTK_EVENT_CONTROLLER(click));

	GtkGesture *drag = gtk_gesture_drag_new();
	g_signal_connect(drag, "drag-update", G_CALLBACK(_drag_cb), NULL);
	gtk_widget_add_controller(g_draw, GTK_EVENT_CONTROLLER(drag));

	g_timeout_add(150, goTick, NULL);

	// 窗口 map 后重发 _NET_WM_STATE（kwin 接管才生效；防 WM 重置置顶/任务栏）
	g_signal_connect(g_win, "map", G_CALLBACK(_map_cb), NULL);
}

// ── X11 底层：move/resize/get_pos/keep_above（GTK4 移除了 gtk_window_move 等）──
static GdkSurface* _surf(void) { return gtk_native_get_surface(GTK_NATIVE(g_win)); }
static Display* _dpy(void) { return GDK_DISPLAY_XDISPLAY(gdk_surface_get_display(_surf())); }
static Window _xid(void) { return gdk_x11_surface_get_xid(GDK_X11_SURFACE(_surf())); }

void hc_move(int x, int y) {
	Display *d = _dpy(); XMoveWindow(d, _xid(), x, y); XFlush(d);
}
void hc_resize(int w, int h) {
	Display *d = _dpy(); XResizeWindow(d, _xid(), (unsigned)w, (unsigned)h); XFlush(d);
}
void hc_get_pos(int *x, int *y) {
	Window c; int rx, ry;
	XTranslateCoordinates(_dpy(), _xid(), DefaultRootWindow(_dpy()), 0, 0, &rx, &ry, &c);
	*x = rx; *y = ry;
}
// ── ctrl 全宽跟踪窗口宽（内层 center 自动居中按钮组）──
void hc_ctrl_fullwidth(int w) {
	if (g_ctrl)
		gtk_widget_set_size_request(g_ctrl, (gint)w, 24);
	if (g_lockbtn) {
		int lx = w - 18 - 6;
		if (lx < 0) lx = 6;
		gtk_fixed_move(GTK_FIXED(g_fixed), g_lockbtn, (double)lx, (double)(116 - 18 - 6));
	}
}


// ── 浮层语义：无条件置顶 + 不占任务栏/Alt-Tab（X11 原生，kwin 可靠）──
// 1) 设 _NET_WM_WINDOW_TYPE = UTILITY（不占任务栏/Alt-Tab/不抢焦点）
// 2) 发 _NET_WM_STATE = ABOVE|SKIP_TASKBAR|SKIP_PAGER|STICKY
// 3) 窗口 map 后 WM 接管会重置状态，故在 map 事件里重发（见 _map_cb）
void hc_keep_above(void) {
	Display *d = _dpy(); Window w = _xid();
	if (!w) return;
	Atom type = XInternAtom(d, "_NET_WM_WINDOW_TYPE", False);
	Atom utility = XInternAtom(d, "_NET_WM_WINDOW_TYPE_UTILITY", False);
	XChangeProperty(d, w, type, XA_ATOM, 32, PropModeReplace, (unsigned char*)&utility, 1);

	Atom net = XInternAtom(d, "_NET_WM_STATE", False);
	Atom above = XInternAtom(d, "_NET_WM_STATE_ABOVE", False);
	Atom skip = XInternAtom(d, "_NET_WM_STATE_SKIP_TASKBAR", False);
	Atom pager = XInternAtom(d, "_NET_WM_STATE_SKIP_PAGER", False);
	Atom sticky = XInternAtom(d, "_NET_WM_STATE_STICKY", False);
	XEvent e; memset(&e, 0, sizeof(e));
	e.xclient.type = ClientMessage; e.xclient.window = w; e.xclient.message_type = net;
	e.xclient.format = 32; e.xclient.data.l[0] = 1; // _NET_WM_STATE_ADD
	e.xclient.data.l[1] = above;
	e.xclient.data.l[2] = skip;
	e.xclient.data.l[3] = pager;
	e.xclient.data.l[4] = sticky;
	XSendEvent(d, DefaultRootWindow(d), False, SubstructureRedirectMask|SubstructureNotifyMask, &e);
	XFlush(d);
}

// 窗口 map 后重发（WM 接管才响应 _NET_WM_STATE；GTK4 无 set_keep_above）
static gboolean _map_cb(GtkWidget *w, GdkEvent *ev, gpointer u) {
	(void)ev; (void)u;
	hc_keep_above();
	return FALSE;
}

// ── X11 真穿透：输入区域用 1bpp pixmap（照 GTK3 版；block=0 全穿透，1 整窗可点）──
void xshape_mask(int block) {
	GdkSurface *surf = _surf();
	if (!surf) return;
	Display *dpy = _dpy();
	Window xid = _xid();
	int w = gtk_widget_get_width(g_win);
	int h = gtk_widget_get_height(g_win);
	int sw = block ? w : 1;
	int sh = block ? h : 1;
	Pixmap pm = XCreatePixmap(dpy, xid, sw, sh, 1);
	GC gc = XCreateGC(dpy, pm, 0, NULL);
	XSetForeground(dpy, gc, block ? 1 : 0);
	XFillRectangle(dpy, pm, gc, 0, 0, sw, sh);
	XShapeCombineMask(dpy, xid, ShapeInput, 0, 0, pm, ShapeSet);
	XFlush(dpy);
	XFreeGC(dpy, gc);
	XFreePixmap(dpy, pm);
}

// ── 鼠标全局坐标（穿透态也有效；照 GTK3 版）──
int pointer_global(int *x, int *y) {
	Display *dpy = XOpenDisplay(NULL);
	if (!dpy) return 0;
	Window root = DefaultRootWindow(dpy);
	Window rr, cc; int rx, ry; unsigned int m;
	int ok = XQueryPointer(dpy, root, &rr, &cc, &rx, &ry, x, y, &m);
	XCloseDisplay(dpy);
	return ok;
}
