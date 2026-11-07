//go:build ios

// iOS 端后端监听：127.0.0.1 仅本机（不暴露局域网；App 内 WebView 加载同端口）
package main

import (
	"log"
	"net/http"
)

func listenBackend(h http.Handler) {
	addr := "127.0.0.1:" + backendPort()
	if err := http.ListenAndServe(addr, logRequest(h)); err != nil {
		log.Printf("[backend] %v", err)
	}
}