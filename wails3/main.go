// wails3 安卓壳（experimental 尝鲜）：内嵌 homecast 全套 Go 服务（模板/静态/API），
// WebView 加载 wails.localhost → 直接走 homecast handler，wails 只当壳。
package main

import (
	"embed"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
	"homecast/server"
)

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	ensureHome() // 安卓：HOME=私有 filesDir（getFilesDir），server.New() 的 ~/.config/homecast 才能落盘
}

func main() {
	app := application.New(application.Options{
		Name:        "HomeCast",
		Description: "家庭音乐播放器（B站音乐 · 个人免费）",
		Assets: application.AssetOptions{
			// homecast 全路由：/ 模板渲染、/assets 静态、/api/v1/*、/hx/* 切片
			Handler: server.New(),
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
