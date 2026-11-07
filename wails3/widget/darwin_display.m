//go:build darwin && !ios

// darwin_display.m — macOS 桌面歌词挂件 ObjC 核心（照 Linux cglue.c / Windows GDI 模式）
// NSWindow 无边框置顶透明 + 穿透(ignoresMouseEvents) + drawRect 绘制 + 鼠标事件
#import <Cocoa/Cocoa.h>

// ── Go 回调（display_darwin.go //export） ──
extern void goOnDraw(int width, int height);          // drawRect：触发 Go 绘制（Go 侧再调 CG）
extern void goOnPress(double x, double y, int button);
extern void goOnRelease(void);
extern void goOnMotion(double x, double y);
extern void _hc_tick_bridge(void);

// ── 全局 ──
static NSWindow *g_win = nil;
static NSTimer *g_timer = nil;
static double g_scale = 1.0;

// HcDrawView：自绘 NSView（drawRect 触发 Go 绘制）
@interface HcDrawView : NSView
@end
@implementation HcDrawView
- (BOOL)isOpaque { return NO; }
- (void)drawRect:(NSRect)r {
    (void)r;
    if (g_win) goOnDraw((int)self.bounds.size.width, (int)self.bounds.size.height);
}
- (void)mouseDown:(NSEvent *)e {
    NSPoint p = [self convertPoint:e.locationInWindow fromView:nil];
    // 屏幕坐标（窗口原点 → 屏幕）
    NSPoint sw = [g_win convertPointToScreen:[self convertPoint:e.locationInWindow fromView:nil]];
    goOnPress(sw.x, sw.y, 0);
}
- (void)mouseUp:(NSEvent *)e { (void)e; goOnRelease(); }
- (void)mouseDragged:(NSEvent *)e {
    NSPoint sw = [g_win convertPointToScreen:[self convertPoint:e.locationInWindow fromView:nil]];
    goOnMotion(sw.x, sw.y);
}
- (void)mouseMoved:(NSEvent *)e {
    NSPoint sw = [g_win convertPointToScreen:[self convertPoint:e.locationInWindow fromView:nil]];
    goOnMotion(sw.x, sw.y);
}
@end

// ── 窗口创建 ──
static HcDrawView *g_view = nil;

int hc_mac_init(int x, int y, int w, int h, double scale) {
    if (g_win) return 0; // 幂等
    __block int ret = 0;
    // NSWindow 必须主线程创建（AppKit 约束）：Go main 协程可能不在主线程，
    // dispatch_sync 主队列保证主线程执行；App 未 run 时主线程空闲，无死锁。
    dispatch_sync(dispatch_get_main_queue(), ^{
        @autoreleasepool {
            g_scale = scale;
            // Go 侧 display_darwin.go 已做 Y 轴翻转（顶部坐标 → 底部坐标），直接传 NSMakeRect
            NSRect frame = NSMakeRect(x, y, w, h);
            NSLog(@"[hcexp] NSMakeRect(%d,%d) %dx%d", x, y, w, h);
            NSWindow *win = [[NSWindow alloc]
                initWithContentRect:frame
                styleMask:NSWindowStyleMaskBorderless
                backing:NSBackingStoreBuffered
                defer:NO];
            [win setOpaque:NO];
            [win setBackgroundColor:[NSColor clearColor]];
            [win setLevel:NSFloatingWindowLevel];     // 置顶
            [win setHasShadow:NO];
            [win setIgnoresMouseEvents:YES];          // 初始穿透
            [win setCollectionBehavior:NSWindowCollectionBehaviorCanJoinAllSpaces | NSWindowCollectionBehaviorFullScreenAuxiliary];
            HcDrawView *v = [[HcDrawView alloc] initWithFrame:frame];
            v.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
            [win setContentView:v];
            [win makeKeyAndOrderFront:nil];
            g_win = win;
            g_view = v;
            ret = 1;
            NSRect af = [win frame];
            NSLog(@"[hcexp] NSWindow created at NSMakeRect(%d,%d) actual frame=%@", x, y, NSStringFromRect(af));
        }
    });
    NSLog(@"[hcexp] hc_mac_init ret=%d", ret);
    return ret;
}

// ── 穿透切换（ShowUI=NO 接收 / HideUI=YES 穿透） ──
void hc_mac_clickable(int on) {
    void (^blk)(void) = ^{
        if (g_win) [g_win setIgnoresMouseEvents:on ? NO : YES];
    };
    if ([NSThread isMainThread]) { blk(); } else { dispatch_sync(dispatch_get_main_queue(), blk); }
}

// ── 窗口操作 ──
int hc_mac_pos(int *px, int *py) {
    __block int ok = 0;
    void (^blk)(void) = ^{
        if (!g_win) return;
        NSRect f = g_win.frame;
        *px = (int)f.origin.x; *py = (int)f.origin.y;
        ok = 1;
    };
    if ([NSThread isMainThread]) { blk(); } else { dispatch_sync(dispatch_get_main_queue(), blk); }
    return ok;
}
void hc_mac_move(int x, int y) {
    void (^blk)(void) = ^{
        if (g_win) [g_win setFrameOrigin:NSMakePoint(x, y)];
    };
    if ([NSThread isMainThread]) { blk(); } else { dispatch_sync(dispatch_get_main_queue(), blk); }
}
void hc_mac_resize(int w, int h) {
    void (^blk)(void) = ^{
        if (g_win) [g_win setContentSize:NSMakeSize(w, h)];
    };
    if ([NSThread isMainThread]) { blk(); } else { dispatch_sync(dispatch_get_main_queue(), blk); }
}
int hc_mac_size(int *w, int *h) {
    __block int ok = 0;
    void (^blk)(void) = ^{
        if (!g_win) return;
        NSRect f = g_win.frame; *w = (int)f.size.width; *h = (int)f.size.height;
        ok = 1;
    };
    if ([NSThread isMainThread]) { blk(); } else { dispatch_sync(dispatch_get_main_queue(), blk); }
    return ok;
}
int hc_mac_screen(int *w, int *h) {
    __block int ok = 0;
    void (^blk)(void) = ^{
        NSRect s = [NSScreen mainScreen].frame;
        *w = (int)s.size.width; *h = (int)s.size.height;
        ok = 1;
    };
    if ([NSThread isMainThread]) { blk(); } else { dispatch_sync(dispatch_get_main_queue(), blk); }
    return ok;
}
static int redraw_count = 0;
void hc_mac_redraw(void) {
    redraw_count++;
    if (redraw_count <= 10 || redraw_count % 50 == 0)
        NSLog(@"[hcexp] redraw #%d", redraw_count);
    if ([NSThread isMainThread]) {
        if (g_view) [g_view setNeedsDisplay:YES];
    } else {
        dispatch_async(dispatch_get_main_queue(), ^{
            if (g_view) [g_view setNeedsDisplay:YES];
        });
    }
}
void hc_mac_visible(int on) {
    void (^blk)(void) = ^{
        if (g_win) { if (on) [g_win orderFront:nil]; else [g_win orderOut:nil]; }
    };
    if ([NSThread isMainThread]) { blk(); } else { dispatch_sync(dispatch_get_main_queue(), blk); }
}

// ── Tick：NSTimer 调度（ObjC 内调 _hc_tick_bridge → Go goTickFn） ──
static int tick_count = 0;
static void hc_tick_fire(NSTimer *t) {
    (void)t;
    tick_count++;
    if (tick_count <= 5 || tick_count % 50 == 0)
        NSLog(@"[hcexp] tick fired #%d", tick_count);
    _hc_tick_bridge();
}
void hc_mac_tick(double ms) {
    NSLog(@"[hcexp] hc_mac_tick ms=%g isMain=%d", ms, [NSThread isMainThread]);
    void (^blk)(void) = ^{
        if (g_timer) [g_timer invalidate];
        g_timer = [NSTimer scheduledTimerWithTimeInterval:ms/1000.0
                     repeats:YES block:^(NSTimer *t){ hc_tick_fire(t); }];
        NSLog(@"[hcexp] NSTimer installed");
    };
    if ([NSThread isMainThread]) { blk(); } else { dispatch_async(dispatch_get_main_queue(), blk); }
}

// ── 鼠标全局坐标（穿透态也有效，同 Windows GetCursorPos） ──
int hc_mac_mouse(int *px, int *py) {
    if ([NSThread isMainThread]) {
        // tick（NSTimer 主线程）→ CursorInside：直接调，不 dispatch_sync（自调用崩溃）
        NSPoint p = [NSEvent mouseLocation];
        *px = (int)p.x; *py = (int)p.y;
        return 1;
    }
    __block int ok = 0;
    dispatch_sync(dispatch_get_main_queue(), ^{
        NSPoint p = [NSEvent mouseLocation];
        *px = (int)p.x; *py = (int)p.y;
        ok = 1;
    });
    return ok;
}

// ── 绘制（drawRect 内：Go Canvas 调；NSGraphicsContext 当前有效） ──
// hc_mac_clear 清透明（无填充——背景透明，直接清）
void hc_mac_clear(void) {
    @autoreleasepool {
        NSRect b = g_view ? g_view.bounds : NSZeroRect;
        [[NSColor clearColor] setFill];
        NSRectFill(b);
    }
}

// hc_mac_text_size 量文字（NSAttributedString size；返回 w,h）
void hc_mac_text_size(const char *text, double size, double *w, double *h) {
    @autoreleasepool {
        NSString *s = [NSString stringWithUTF8String:text ? text : ""];
        NSFont *f = [NSFont systemFontOfSize:size];
        NSDictionary *attrs = @{ NSFontAttributeName: f };
        NSSize sz = [s sizeWithAttributes:attrs];
        *w = sz.width; *h = sz.height;
    }
}

// hc_mac_text 画文字（Go 传左上角坐标，NSView Y轴向上需翻转：y_top → y_bottom）
void hc_mac_text(double x, double y, const char *text, double size, double r, double g, double b, double a) {
    @autoreleasepool {
        NSString *s = [NSString stringWithUTF8String:text ? text : ""];
        NSFont *f = [NSFont systemFontOfSize:size];
        NSColor *col = [NSColor colorWithCalibratedRed:r green:g blue:b alpha:a];
        NSDictionary *attrs = @{ NSFontAttributeName: f, NSForegroundColorAttributeName: col };
        // drawRect 内：当前 NSGraphicsContext 有效（Go paint 调）
        // Y 轴翻转：Go 的 y 是从顶部算（Linux/Windows 一致），NSView Y 轴向上
        NSRect b = g_view ? g_view.bounds : NSZeroRect;
        double yBottom = b.size.height - y - size;
        [s drawAtPoint:NSMakePoint(x, yBottom) withAttributes:attrs];
    }
}

// ── 挂件退出 ──
void hc_mac_quit(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        @autoreleasepool {
            if (g_timer) [g_timer invalidate];
            if (g_win) [g_win orderOut:nil];
            [NSApp terminate:nil];
        }
    });
}

// ── 主循环（NSApplication run；由 Go 调，主线程） ──
void hc_mac_run(void) {
    NSLog(@"[hcexp] dispatch_async NSApp run from thread=%d", [NSThread isMainThread]);
    // [NSApp run] 必须主线程；后台 goroutine 调用时 dispatch_async 到主线程
    if ([NSThread isMainThread]) {
        [NSApp run];
    } else {
        dispatch_async(dispatch_get_main_queue(), ^{
            NSLog(@"[hcexp] NSApp run start (main)");
            [NSApp run];
        });
    }
}

// hc_mac_fill_rect 填充矩形（drawRect 内 NSGraphicsContext 有效；进度条/按钮背景/聚焦框）
// Go 传顶部坐标，NSView Y 轴向上需翻转：y_top → y_bottom = viewH - y - h
void hc_mac_fill_rect(double x, double y, double w, double h, double r, double g, double b, double a) {
    @autoreleasepool {
        NSColor *col = [NSColor colorWithCalibratedRed:r green:g blue:b alpha:a];
        [col setFill];
        NSRect b = g_view ? g_view.bounds : NSZeroRect;
        double yBottom = b.size.height - y - h;
        NSRectFill(NSMakeRect(x, yBottom, w, h));
    }
}
