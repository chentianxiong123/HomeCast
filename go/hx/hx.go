// Package hx 功能切片层：一个功能一个文件（面向过程，直上直下）
//
// 壳（本文件）：页面渲染（shell 注入 content）+ 路由分发
// 页面结构对照 Vue 版：/hx/music 音乐 /hx/search 搜索 /hx/favs 收藏夹 /hx/cast 投屏
package hx

import (
	"bytes"
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

// page 页面渲染数据：激活 tab + 预渲染的 content HTML
// （Go 模板不支持 {{template .Content .}} 动态名，先渲染 content 再注入 shell）
type page struct {
	Active  string        // nav tab key: music / search / favs / cast
	Content template.HTML // content 模板渲染结果
}

// renderPage 渲染完整页面（shell + content），tab 高亮由服务端决定（多页天然正确）
func (h *H) renderPage(w http.ResponseWriter, active, content string, data any) {
	var buf bytes.Buffer
	if err := h.Tpl.ExecuteTemplate(&buf, content, data); err != nil {
		http.Error(w, "render: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.Tpl.ExecuteTemplate(w, "shell.html", &page{Active: active, Content: template.HTML(buf.String())})
}

// Router 组装 hx 路由（挂在 server 外层 mux 的 "/" 上）
func (h *H) Router() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /assets/", http.FileServerFS(web.Assets()))
	// 页面（对照 Vue 版 nav：音乐/搜索/收藏夹/投屏）
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { h.searchPage(w, r) })
	mux.HandleFunc("GET /hx/music", h.musicPage)
	mux.HandleFunc("GET /hx/search", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.URL.Query()["kw"]; ok {
			h.Search(w, r) // 带 kw = 结果片段
		} else {
			h.searchPage(w, r) // 无 kw = 搜索页
		}
	})
	mux.HandleFunc("GET /hx/favs", h.favsPage)
	mux.HandleFunc("GET /hx/cast", h.castPage)
	return mux
}

// searchPage 搜索页（含搜索框 + 空态）
func (h *H) searchPage(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, "search", "content_search.html", &searchPageData{})
}

// musicPage 音乐页（播放列表，M2 实现，当前占位）
func (h *H) musicPage(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, "music", "content_music.html", nil)
}

// favsPage 收藏夹页（M2 实现，当前占位）
func (h *H) favsPage(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, "favs", "content_favs.html", nil)
}

// castPage 投屏页（M4 实现，当前占位）
func (h *H) castPage(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, "cast", "content_cast.html", nil)
}