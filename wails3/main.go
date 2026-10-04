// HomeCast 统一壳（wails3 v3）：一个入口，三端构建
//
//	🐧 Linux / 🪟 Windows / 🤖 安卓 共用：
//	  - 内嵌 go/ 后端（server.New() handler，模板/静态/API/SQLite 全在内）
//	  - wails WebView 窗口直载 htmx 界面（Assets.Handler = logRequest(injectHcEnv(h))）
//	  - 桌面端（Linux/Windows）：额外 0.0.0.0 HTTP 服务（局域网 DLNA/音箱/挂件拉歌词）+ 桌面歌词挂件
//	  - 安卓端：独立监听/挂件均 no-op（悬浮窗歌词挂件待写）
//	平台差异全部走同目录 build tag 文件：
//	  ensurehome_android.go / ensurehome_other.go   —— 数据目录（安卓 filesDir vs 桌面 ~/.config）
//	  http_desktop.go / http_android.go             —— 局域网监听 + hcEnv 注入
//	  widget_desktop.go / widget_android.go         —— 桌面歌词挂件启动
package main

import (
	"embed"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"homecast/server"
)

//go:embed all:frontend/dist
var assets embed.FS

// httpLog 把每个请求的状态/panic 写进文件（安卓 stderr 不可见，文件可取证）
var httpLog string

var app *application.App

// desktopEventsBridge widget → 前端事件桥（wails 事件系统）。
// 类型定义放公共文件（main.go）：main() 调用 startWidget 时引用，widget 包只在桌面端编译。
type desktopEventsBridge struct{}

func (desktopEventsBridge) Emit(cmd string, payload ...any) {
	if app != nil {
		app.Event.Emit("widget-cmd", append([]any{cmd}, payload...)...)
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
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

func main() {
	ensureHome() // 必须在 main()（nativeInit 之后，JNI bridge 可用时）调用；init() 里拿不到 StoragePath
	httpLog = filepath.Join(hcDataPath(), "hc-http.log")
	os.WriteFile(httpLog, []byte("HOME="+os.Getenv("HOME")+"\n"), 0644)

	h := server.New()

	// 桌面端：0.0.0.0 独立端口让局域网 DLNA/音箱/挂件能访问代理流；安卓 no-op
	go listenBackend(h)

	app = application.New(application.Options{
		Name:        "HomeCast",
		Description: "家庭音乐播放器（B站音乐 · 个人免费）",
		Assets: application.AssetOptions{
			// homecast 全路由：/ 模板渲染、/assets 静态、/api/v1/*、/hx/* 切片
			Handler: logRequest(injectHcEnv(h)),
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "HomeCast 家庭投屏播放器",
		Width:            1280,
		Height:           800,
		MinWidth:         920,
		MinHeight:        620,
		BackgroundColour: application.NewRGB(17, 17, 17),
		URL:              "/",
	})

	// 桌面歌词挂件（Linux=GTK / Windows=Win32 显示层）；安卓 no-op（悬浮窗待写）
	if os.Getenv("HC_NO_WIDGET") == "" {
		startWidget(server.WidgetStateSnapshot, desktopEventsBridge{}, "http://127.0.0.1:"+backendPort())
	}

	if err := app.Run(); err != nil {
		os.Exit(1)
	}
}