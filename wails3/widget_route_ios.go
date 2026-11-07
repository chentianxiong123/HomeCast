//go:build ios

// iOS 挂件控制不走 HTTP（iOS 无桌面挂件，纯 App 内播放器）。no-op（不碰 widget 包）。
package main

import "net/http"

func wrapWidgetRoute(h http.Handler) http.Handler {
	return h
}