// Package hx 功能切片层：一个功能一个文件（面向过程，直上直下）
//
// 壳（本文件）：页面渲染（shell 注入 content）+ 路由分发
// 页面结构（对齐 YesPlayMusic 精简模型）：/hx/queue 队列 /hx/search 搜索 /hx/favs 收藏夹 /hx/cast 投屏
package hx

import (
	"bytes"
	"html/template"
	"io/fs"
	"net/http"

	"homecast/internal/service"
	"homecast/internal/speaker"
	"homecast/web"
)

// H 功能上下文：只有各功能要用的数据句柄，无接口无抽象
type H struct {
	Music   *service.MusicService
	Fav     *service.FavService
	Cast    *service.CastService
	Speaker *service.SpeakerService
	QR      *speaker.QRLogin
	Tpl     *template.Template
}

// page 页面渲染数据：激活 tab + 预渲染的 content HTML
// （Go 模板不支持 {{template .Content .}} 动态名，先渲染 content 再注入 shell）
type page struct {
	Active  string        // nav tab key: music / search / favs / cast
	Content template.HTML // content 模板渲染结果
}

// renderPage 渲染完整页面（shell + content），tab 高亮由 nav.js 按 URL 推导
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
	// 静态资源 no-store：本地开发异步加载，浏览器缓存会造成「我改了你没看到」的错位
	mux.Handle("GET /assets/", assetHandler(web.Assets()))
	// 页面（对照 Vue 版 nav：音乐/搜索/收藏夹/投屏）
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { h.searchPage(w, r) })
	mux.HandleFunc("GET /hx/queue", h.QueuePage)
	mux.HandleFunc("GET /hx/recent", h.RecentPage)
	mux.HandleFunc("GET /hx/search", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.URL.Query()["kw"]; ok {
			h.Search(w, r) // 带 kw = 结果片段或完整页（按 HX-Request 区分）
		} else {
			h.searchPage(w, r) // 无 kw = 搜索页
		}
	})
	mux.HandleFunc("GET /hx/favs", h.FavsPage)
	mux.HandleFunc("GET /hx/cast", h.CastPage)
	mux.HandleFunc("GET /hx/settings", h.SettingsPage)
	mux.HandleFunc("POST /hx/cast/rescan", h.Rescan)
	mux.HandleFunc("POST /hx/cast/play", h.CastPlay)
	mux.HandleFunc("GET /hx/cast/status/{udn}", h.CastStatus)
	// 音箱切面（M4b）
	mux.HandleFunc("GET /hx/speaker", h.SpeakerSection)
	mux.HandleFunc("POST /hx/speaker/qr-start", h.SpeakerQRStart)
	mux.HandleFunc("GET /hx/speaker/qr-status", h.SpeakerQRStatus)
	mux.HandleFunc("POST /hx/speaker/play", h.SpeakerPlay)
	mux.HandleFunc("POST /hx/speaker/control", h.SpeakerControl)
	mux.HandleFunc("POST /hx/speaker/refresh", h.SpeakerRefresh)
	// 播放列表切面
	// 收藏切面
	mux.HandleFunc("POST /hx/fav/toggle", h.FavToggle)
	mux.HandleFunc("POST /hx/fav/remove/{bvid}", h.FavRemove)
	mux.HandleFunc("POST /hx/fav/clear", h.FavClear)
	return mux
}

// assetHandler 静态资源包装：HTTP 返回强制不缓存（asset 治理为服务端最新）
func assetHandler(assets fs.FS) http.Handler {
	fsrv := http.FileServerFS(assets)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		fsrv.ServeHTTP(w, r)
	})
}

// SettingsPage GET /hx/settings → 设置页（P 态开关集中可见可改，settings.js 渲染）
func (h *H) SettingsPage(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, "settings", "content_settings.html", nil)
}

// QueuePage GET /hx/queue → 播放队列（自动上下文，前端收 hc:queue 渲染）
func (h *H) QueuePage(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, "queue", "content_queue.html", nil)
}

// RecentPage GET /hx/recent → 最近播放全页面（前端读 hc:recent 渲染全部）
func (h *H) RecentPage(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, "recent", "content_recent.html", nil)
}

// searchPage 搜索页（含搜索框 + 空态）
func (h *H) searchPage(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, "search", "content_search.html", &searchPageData{})
}

