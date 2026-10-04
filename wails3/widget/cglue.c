#ifndef _WIN32
// C 实现层：GTK 挂件窗口的 C 侧代码（独立编译单元，避免 preamble 重复定义）
// C 实现层：GTK 挂件窗口的 C 侧代码（独立编译单元，避免 preamble 重复定义）
#include <gtk/gtk.h>
#include <gdk/gdkx.h>
#include <X11/Xlib.h>
#include <X11/extensions/shape.h>

// Go 侧回调（//export 生成于 _cgo_export.c）
extern gboolean goOnDraw(GtkWidget *w, cairo_t *cr, gpointer user);
extern gboolean goOnButtonPress(GtkWidget *w, GdkEventButton *e, gpointer u);
extern gboolean goOnButtonRelease(GtkWidget *w, GdkEventButton *e, gpointer u);
extern gboolean goOnMotion(GtkWidget *w, GdkEventMotion *e, gpointer u);
extern void goOnPauseClick(void);
extern void goOnMinusClick(void);
extern void goOnPlusClick(void);
extern void goOnLockClick(void);
extern void goOnCloseClick(void);
extern gboolean goTick(gpointer user);

static GtkWidget *g_win = NULL;
static GtkWidget *g_fixed = NULL;
static GtkWidget *g_ctrl = NULL;
static GtkWidget *g_lockbtn = NULL;
static GdkWindow *g_win_gdk = NULL;

GtkWidget* hc_win(void)       { return g_win; }
GtkWidget* hc_ctrl(void)      { return g_ctrl; }
GtkWidget* hc_lockbtn(void)   { return g_lockbtn; }
GtkWidget* hc_fixed(void)     { return g_fixed; }
GdkWindow* hc_win_gdk(void)   { return g_win_gdk; }
void hc_set_win(GtkWidget *w)     { g_win = w; }
void hc_set_ctrl(GtkWidget *w)    { g_ctrl = w; }
void hc_set_lockbtn(GtkWidget *w) { g_lockbtn = w; }
void hc_set_fixed(GtkWidget *w)   { g_fixed = w; }
void hc_set_win_gdk(GdkWindow *w) { g_win_gdk = w; }

static void _hc_btn_cb(GtkWidget *b, gpointer d) {
	int kind = GPOINTER_TO_INT(d);
	switch (kind) {
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

void _hc_css(void) {
	GtkCssProvider *p = gtk_css_provider_new();
	const char *css =
		"#hc-btn, #hc-lock { background: rgba(20,20,30,130); color: #d8d8e8;"
		" border: none; border-radius: 4px; font-size: 11px; padding: 0px; min-height: 0px;"
		" box-shadow: none; outline: none; }"
		"#hc-btn:hover, #hc-lock:hover { background: rgba(255,255,255,70); color: #ffffff; }"
		"#hc-ctrl { background: transparent; }";
	gtk_css_provider_load_from_data(p, css, -1, NULL);
	GdkScreen *s = gdk_screen_get_default();
	gtk_style_context_add_provider_for_screen(s, GTK_STYLE_PROVIDER(p),
		GTK_STYLE_PROVIDER_PRIORITY_APPLICATION);
	g_object_unref(p);
}

void wire_signals(void) {
	g_signal_connect(g_win, "draw", G_CALLBACK(goOnDraw), NULL);
	g_signal_connect(g_win, "button-press-event", G_CALLBACK(goOnButtonPress), NULL);
	g_signal_connect(g_win, "button-release-event", G_CALLBACK(goOnButtonRelease), NULL);
	g_signal_connect(g_win, "motion-notify-event", G_CALLBACK(goOnMotion), NULL);
	g_timeout_add(150, goTick, NULL);
}

// X11 真穿透：输入区域用 1bpp pixmap（block=0 全穿透，1 整窗可点）
void xshape_mask(int block) {
	Display *dpy = XOpenDisplay(NULL);
	if (!dpy || !g_win_gdk) return;
	Window xid = gdk_x11_window_get_xid(g_win_gdk);
	int w = block ? gdk_window_get_width(g_win_gdk) : 1;
	int h = block ? gdk_window_get_height(g_win_gdk) : 1;
	Pixmap pm = XCreatePixmap(dpy, xid, w, h, 1);
	GC gc = XCreateGC(dpy, pm, 0, NULL);
	XSetForeground(dpy, gc, block ? 1 : 0);
	XFillRectangle(dpy, pm, gc, 0, 0, w, h);
	XShapeCombineMask(dpy, xid, ShapeInput, 0, 0, pm, ShapeSet);
	XFlush(dpy);
	XFreeGC(dpy, gc);
	XFreePixmap(dpy, pm);
	XCloseDisplay(dpy);
}

// 鼠标全局坐标（穿透态也有效）
int pointer_global(int *x, int *y) {
	Display *dpy = XOpenDisplay(NULL);
	if (!dpy) return 0;
	Window root = DefaultRootWindow(dpy);
	Window rr, cc; int rx, ry; unsigned int m;
	int ok = XQueryPointer(dpy, root, &rr, &cc, &rx, &ry, x, y, &m);
	XCloseDisplay(dpy);
	return ok;
}
#endif // _WIN32
