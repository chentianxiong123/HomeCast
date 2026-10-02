// homecast 桌面壳（Wails v2）：内嵌全部 Go 后端 + 窗口直载 htmx 页面
// （AssetServer 代理 wails:// → 内嵌后端，Vue 前端已废弃停用）
// + 桌面歌词挂件窗口（同进程，GTK3）
package main

import (
	"context"
	"log"
	"net/http/httputil"
	"net/url"
	"os"

	"homecast-desktop/widget"
	"homecast/server"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// wailEvents EventsEmit 包装（挂件 → 前端）
type wailEvents struct{}

func (wailEvents) Emit(cmd string, payload ...any) {
	if runtimeApp == nil {
		return
	}
	runtime.EventsEmit(runtimeApp, "widget-cmd", append([]any{cmd}, payload...)...)
}

var runtimeApp context.Context

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

	// 壳窗口直载 htmx 页：wails:// 所有请求代理到内嵌后端（同源，无跨域）
	backend, _ := url.Parse("http://127.0.0.1:" + port)
	proxy := httputil.NewSingleHostReverseProxy(backend)

	app := NewApp(port)
	err := wails.Run(&options.App{
		Title:     "HomeCast 家庭投屏播放器",
		Width:     1280,
		Height:    800,
		MinWidth:  920,
		MinHeight: 620,
		AssetServer: &assetserver.Options{
			Handler: proxy, // 直载 htmx 页（不再用 Vue dist）
		},
		OnStartup: func(ctx context.Context) {
			runtimeApp = ctx
			// 桌面歌词挂件（主线程内创建 GTK 窗口）
			if os.Getenv("HC_NO_WIDGET") == "" {
				if err := widget.Start(server.WidgetStateSnapshot, wailEvents{}, "http://127.0.0.1:"+port); err != nil {
					log.Printf("[widget] 启动失败: %v", err)
				}
			}
		},
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}