// Package hx 功能切片层：一个功能一个文件（面向过程，直上直下）
//
// 壳（本文件）：渲染首页骨架 + 路由分发（/assets 静态 + /hx/* 切片 + / 首页）
package hx

import (
	"html/template"
	"net/http"

	"homecast/internal/service"
	"homecast/web"
)

// H 功能上下文：只有各功能要用的数据句柄，无接口无抽象
type H struct {
	Music *service.MusicService
	Tpl   *template.Template
}

// Router 组装 hx 路由（挂在 server 外层 mux 的 "/" 上）
func (h *H) Router() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /assets/", http.FileServerFS(web.Assets()))
	mux.HandleFunc("GET /hx/search", h.Search)
	mux.HandleFunc("/", h.Index) // 兜底：首页（未匹配的路径渲染先骨架）
	return mux
}

// Index 首页壳：导航 + 搜索容器 + 播放器 dock 占位
func (h *H) Index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.Tpl.ExecuteTemplate(w, "index.html", nil)
}