//go:build !android

package main

// 桌面端（Linux/Windows）后端监听：0.0.0.0 让局域网 DLNA / 小米音箱 / 挂件能访问代理流
// （迁移后这是唯一 HTTP 服务，WebView 也走它：http://127.0.0.1:端口）

import (
	"log"
	"net/http"
)

// listenBackend 0.0.0.0 单端口起后端
func listenBackend(h http.Handler) {
	addr := "0.0.0.0:" + backendPort()
	if err := http.ListenAndServe(addr, h); err != nil {
		log.Printf("[backend] %v", err)
	}
}
