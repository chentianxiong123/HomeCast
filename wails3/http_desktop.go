//go:build !android

package main

// 桌面端（Linux/Windows）HTTP 增强：
//  1. 0.0.0.0 独立端口起后端 —— 局域网 DLNA 投屏 / 小米音箱 / 挂件能访问代理流
//  2. hcEnv 注入 —— shell.html 用它区分桌面/网页版（有 hcEnv 则 htmx 不补 hc=1 参数）

import (
	"bytes"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// listenBackend 0.0.0.0 让局域网音箱/DLNA 设备能访问代理流地址
func listenBackend(h http.Handler) {
	addr := "0.0.0.0:" + backendPort()
	if err := http.ListenAndServe(addr, h); err != nil {
		log.Printf("[backend] %v", err)
	}
}

// injectHcEnv 给 text/html 响应追加 window.hcEnv='desktop'（同源注入，无跨域）
func injectHcEnv(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &recorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		body := rec.buf.Bytes()
		if strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
			body = append(body, []byte("<script>window.hcEnv='desktop'</script>")...)
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		}
		// 非 html：Content-Length 保持原 handler 设置的，body 原样
		w.Write(body)
	})
}

// recorder 全量缓冲响应（挂件/窗口直载流量；与旧 proxy.ModifyResponse 行为等价）
type recorder struct {
	http.ResponseWriter
	buf bytes.Buffer
}

func (r *recorder) Write(p []byte) (int, error) { return r.buf.Write(p) }