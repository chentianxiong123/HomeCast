//go:build android

package main

// 安卓端 HTTP 增强 no-op：
//  - 不做 0.0.0.0 独立端口（wails WebView 直走 server.New() handler）
//  - 不注入 hcEnv（安卓 asset 桥丢 header，前端用 hc=1 参数代替 HX-Request）

import "net/http"

func listenBackend(_ http.Handler) {}

func injectHcEnv(h http.Handler) http.Handler { return h }