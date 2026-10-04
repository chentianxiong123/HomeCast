// wails3 安卓壳（experimental 尝鲜）：内嵌 homecast 全套 Go 服务（模板/静态/API），
// WebView 加载 wails.localhost → 直接走 homecast handler，wails 只当壳。
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
			if p := recover(); p != nil {
				msg := fmt.Sprintf("PANIC %s %s -> %v\n%s\n", r.Method, r.URL.Path, p, debug.Stack())
				os.WriteFile(httpLog, []byte(msg), 0644)
				sw.WriteHeader(500)
			} else {
				msg := fmt.Sprintf("%s %s -> %d (%s) via %s\n", r.Method, r.URL.Path, sw.status, time.Since(start), r.RemoteAddr)
				os.WriteFile(httpLog, []byte(msg), 0644)
			}
		}()
		h.ServeHTTP(sw, r)
	})
}

func main() {
	ensureHome() // 必须在 main()（nativeInit 之后，JNI bridge 可用时）调用；init() 里拿不到 StoragePath
	httpLog = filepath.Join(application.Android.StoragePath(), "hc-http.log")
	os.WriteFile(httpLog, []byte("HOME="+os.Getenv("HOME")+"\n"), 0644)

	app := application.New(application.Options{
		Name:        "HomeCast",
		Description: "家庭音乐播放器（B站音乐 · 个人免费）",
		Assets: application.AssetOptions{
			// homecast 全路由：/ 模板渲染、/assets 静态、/api/v1/*、/hx/* 切片
			Handler: logRequest(server.New()),
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "HomeCast",
		Width:            1000,
		Height:           700,
		BackgroundColour: application.NewRGB(17, 17, 17),
		URL:              "/",
	})

	if err := app.Run(); err != nil {
		os.Exit(1)
	}
}