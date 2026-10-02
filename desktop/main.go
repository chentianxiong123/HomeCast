// homecast 桌面壳（Wails v2）：内嵌全部 Go 后端 + 加载 Vue 前端构建产物
package main

import (
	"embed"
	"log"
	"os"

	"homecast/server"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:dist
var assets embed.FS

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

	app := NewApp(port)
	err := wails.Run(&options.App{
		Title:     "HomeCast 家庭投屏播放器",
		Width:     1280,
		Height:    800,
		MinWidth:  920,
		MinHeight: 620,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}