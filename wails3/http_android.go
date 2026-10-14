//go:build android

package main

// 安卓端后端监听：127.0.0.1 仅本机（不暴露局域网）；挂件/局域网均 no-op（悬浮窗待写）

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