// HomeCast 统一壳（wails3 v3）：一个入口，三端构建
//
//	🐧 Linux / 🪟 Windows / 🤖 安卓 共用：
//	  - 内嵌 go/ 后端（server.New() handler，模板/静态/API/SQLite 全在内）
//	  - wails WebView 窗口直载 htmx 界面：URL 指向后端真实 HTTP 端口（同源，非 wails asset 桥）
//	  - 桌面端（Linux/Windows）：后端监听 0.0.0.0（局域网 DLNA/音箱/挂件拉歌词）+ 桌面歌词挂件
//	  - 安卓端：后端监听 127.0.0.1 + 挂件 no-op（悬浮窗歌词挂件待写）
//	平台差异全部走同目录 build tag 文件：
//	  ensurehome_android.go / ensurehome_other.go   —— 数据目录（安卓 filesDir vs 桌面 ~/.config）
//	  http_desktop.go / http_android.go             —— 后端监听地址（0.0.0.0 vs 127.0.0.1）
//	  widget_desktop.go / widget_android.go         —— 桌面歌词挂件启动
//
//	迁移见 docs/2026-10-14-wails系统壳迁移计划.md（wails 只做系统壳，业务走真实 HTTP）
package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"homecast/server"
)

// httpLog 把每个请求的状态/panic 写进文件（安卓 stderr 不可见，文件可取证）
var httpLog string

var app *application.App

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Flush 让 statusWriter 支持 http.Flusher（SSE 需要；否则下游断言 Flusher 失败）
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func logRequest(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w, status: 200}
		start := time.Now()
		defer func() {
			var msg string
			if p := recover(); p != nil {
				msg = fmt.Sprintf("PANIC %s %s -> %v\n%s\n", r.Method, r.URL.Path, p, debug.Stack())
				sw.WriteHeader(500)
			} else {
				msg = fmt.Sprintf("%s %s -> %d (%s)\n", r.Method, r.URL.Path, sw.status, time.Since(start))
			}
			f, err := os.OpenFile(httpLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err == nil {
				f.WriteString(msg)
				f.Close()
			}
		}()
		h.ServeHTTP(sw, r)
	})
}

// backendPort 独立 HTTP 端口（挂件/局域网访问）；HC_PORT 覆盖
func backendPort() string {
	if p := os.Getenv("HC_PORT"); p != "" {
		return p
	}
	return server.Port
}

// musicIcon 生成一个 🎵 音符 PNG 图标（16x16，紫色底 + 白色音符点），
// 替换 wails 默认应用 logo；application.Options.Icon 用于任务栏/about。
func musicIcon() []byte {
	const s = 16
	img := image.NewRGBA(image.Rect(0, 0, s, s))
	// 紫色圆角底
	for y := 0; y < s; y++ {
		for x := 0; x < s; x++ {
			img.Set(x, y, color.RGBA{0x88, 0x33, 0x99, 255})
		}
	}
	// 白色音符：竖杆 + 两个符头（简单像素音符）
	for y := 2; y < 11; y++ { img.Set(6, y, color.RGBA{255, 255, 255, 255}) }
	for y := 9; y < 13; y++ { for x := 3; x < 6; x++ { img.Set(x, y, color.RGBA{255, 255, 255, 255}) } }
	for y := 6; y < 10; y++ { for x := 9; x < 12; x++ { img.Set(x, y, color.RGBA{255, 255, 255, 255}) } }
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func main() {
	ensureHome() // 必须在 main()（nativeInit 之后，JNI bridge 可用时）调用；init() 里拿不到 StoragePath
	httpLog = filepath.Join(hcDataPath(), "hc-http.log")
	os.WriteFile(httpLog, []byte("HOME="+os.Getenv("HOME")+"\n"), 0644)

	h := wrapWidgetRoute(server.New())

	// 桌面端：0.0.0.0 独立端口让局域网 DLNA/音箱/挂件能访问代理流；安卓 no-op
	go listenBackend(h)

	// 阶段3：安卓端也走真实 HTTP（MainActivity 加载 http://127.0.0.1:28976，见
	// build/android/.../MainActivity.java BACKEND_URL），不再走 wails asset 桥。
	// 因此不需要 Assets.Handler（桌面走绝对 URL 也不走 asset 桥）。asset 桥彻底退役。
	app = application.New(application.Options{
		Name:        "HomeCast",
		Description: "家庭音乐播放器（B站音乐 · 个人免费）",
		Icon:        musicIcon(), // 🎵 应用图标（替换 wails 默认 logo）
	})

	// WebviewWindowOptions.JS：页面加载完成后由 wails 官方注入（替代原 injectHcEnv 中间件）。
	// 前端 settings.js/subtitle.js 读 window.hcEnv 区分桌面壳/网页，业务用途保留。
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "HomeCast 家庭投屏播放器",
		Width:            1280,
		Height:           800,
		MinWidth:         920,
		MinHeight:        620,
		BackgroundColour: application.NewRGB(17, 17, 17),
		URL:              "http://127.0.0.1:" + backendPort(),
		JS:               "window.hcEnv='desktop';",
	})

	// 桌面歌词挂件（Linux=GTK / Windows=Win32 显示层）；安卓 no-op（悬浮窗待写）
	if os.Getenv("HC_NO_WIDGET") == "" {
		startWidget(server.WidgetStateSnapshot, "http://127.0.0.1:"+backendPort())
	}

	if err := app.Run(); err != nil {
		os.Exit(1)
	}
}