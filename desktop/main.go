// homecast 桌面壳（Wails v3）：内嵌全部 Go 后端 + 窗口直载 htmx 页面 + 桌面歌词挂件（GTK3）
// 与 v2 行为等价：端口后端保留（挂件拉歌词、局域网音箱/DLNA 访问代理流地址）、
// wails asset → 反向代理 → 内嵌后端（同源）+ 注入 window.hcEnv='desktop'。
package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"homecast/server"
	"homecast-desktop/widget"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var app *application.App

// desktopEvents widget → 前端事件桥（wails 事件系统）
type desktopEvents struct{}

func (desktopEvents) Emit(cmd string, payload ...any) {
	if app != nil {
		app.Event.Emit("widget-cmd", append([]any{cmd}, payload...)...)
	}
}

func main() {
	port := os.Getenv("HC_PORT")
	if port == "" {
		port = server.Port
	}

	// 内嵌后端：0.0.0.0 让局域网音箱/DLNA 设备能访问代理流地址
	go func() {
		if err := server.ListenAndServe("0.0.0.0:" + port); err != nil {
			log.Printf("[backend] %v", err)
		}
	}()

	// 壳窗口直载 htmx 页：asset 请求代理到内嵌后端（同源，无跨域）
	backend, _ := url.Parse("http://127.0.0.1:" + port)
	proxy := httputil.NewSingleHostReverseProxy(backend)
	proxy.ModifyResponse = func(resp *http.Response) error {
		if strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			inject := "<script>window.hcEnv='desktop'</script>"
			body = append(body, []byte(inject)...)
			resp.Body = io.NopCloser(bytes.NewReader(body))
			resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
		}
		return nil
	}

	app = application.New(application.Options{
		Name:        "HomeCast",
		Description: "家庭投屏播放器（B站音乐 · 个人免费）",
		Assets: application.AssetOptions{
			Handler: proxy,
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

	// 桌面歌词挂件（GTK3 独立组件，信号回调由 v3 主循环驱动）
	if os.Getenv("HC_NO_WIDGET") == "" {
		time.AfterFunc(time.Second, func() {
			if err := widget.Start(server.WidgetStateSnapshot, desktopEvents{}, "http://127.0.0.1:"+port); err != nil {
				log.Printf("[widget] 启动失败: %v", err)
			}
		})
	}

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}