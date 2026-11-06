//go:build android

// 安卓挂件控制不走 HTTP：下面面板 #d-tick 走 window.AndroidLyric 桥（悬浮窗 Service），
// 不经过 /api/v1/widget/lyric-visible（该路由只在桌面壳层注册）。安卓 no-op（不碰 widget 包）。
package main

import "net/http"

func wrapWidgetRoute(h http.Handler) http.Handler {
	return h
}
