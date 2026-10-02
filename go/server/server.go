// Package server 组装 homecast 全部后端 handler（HTTP 服务）
// 被 cmd/server（独立进程）和 desktop（Wails 壳内嵌）复用
package server

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"homecast/hx"
	"homecast/internal/api"
	"homecast/internal/bilibili"
	"homecast/internal/service"
	"homecast/internal/speaker"
	"homecast/internal/store"
	"homecast/web"
)

// Port 默认端口：Go 版独立端口，与 Python 28974 并存对照
const Port = "28976"

// New 组装全部 handler（带 CORS，桌面壳跨域用）
func New() http.Handler {
	client := bilibili.NewClient(
		"https://api.bilibili.com",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"https://search.bilibili.com/", // 对齐 Python 默认值！www.bilibili.com 会被风控返回 HTML
		30*time.Second,
	)

	// SQLite 单文件持久层：首启自动从旧 JSON 迁移（JSON 保留只读备份）
	home, _ := os.UserHomeDir()
	dataDir := filepath.Join(home, ".config", "homecast")
	db, err := store.Open(filepath.Join(dataDir, "homecast.db"))
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, dataDir); err != nil {
		log.Fatalf("migrate db: %v", err)
	}

	musicSvc := &service.MusicService{Client: client}
	musicHandler := api.NewMusicHandler(musicSvc)
	favHandler := &api.FavHandler{Fav: service.NewFavService(db)}
	lyricHandler := &api.LyricHandler{}
	playlistHandler := &api.PlaylistHandler{Playlist: service.NewPlaylistService(client, db)}
	tokenStore := service.NewTokenStore()
	castHandler := &api.CastHandler{Cast: service.NewCastService(client, tokenStore)}
	proxyHandler := &api.ProxyHandler{Tokens: tokenStore}
	speakerSvc := service.NewSpeakerService(speaker.NewSpeakerAuth(), client, tokenStore)
	speakerHandler := &api.SpeakerHandler{Svc: speakerSvc, QR: speaker.NewQRLogin()}
	sitesHandler := &api.SitesHandler{Sites: service.NewSitesService("")}
	widgetState := service.NewWidgetState()
	widgetStateGlobal = widgetState
	widgetHandler := &api.WidgetHandler{State: widgetState}

	mux := api.NewMux(
		musicHandler, favHandler, lyricHandler, playlistHandler,
		castHandler, proxyHandler, speakerHandler, sitesHandler, widgetHandler,
	)

	// htmx 层（Go 渲染页面 + 功能切片）：/ 首页、/assets 静态、/hx/* 切片
	hxH := &hx.H{Music: musicSvc, Fav: service.NewFavService(db), PL: service.NewPlaylistService(client, db), Tpl: web.MustTemplates()}
	outer := http.NewServeMux()
	outer.Handle("/", hxH.Router())
	outer.Handle("/api/", mux)
	return withCORS(outer)
}

// WidgetStateRef 挂件（同进程）读取播放状态用
func WidgetStateRef() *service.WidgetState { return widgetStateGlobal }

// WidgetStateSnapshot 公开快照（desktop/widget 轮询用，不暴露内部类型）
func WidgetStateSnapshot() (bvid, title, artist string, currentTime, duration float64, playing bool) {
	if widgetStateGlobal != nil {
		return widgetStateGlobal.Get()
	}
	return "", "", "", 0, 0, false
}

var widgetStateGlobal *service.WidgetState

// withCORS 允许任意来源（本地单用户服务；桌面壳 wails:// 域跨域访问）
func withCORS(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			log.Printf("[api] %s %s", r.Method, r.URL.Path)
		}
		h.ServeHTTP(w, r)
	})
}

// ListenAndServe 启动服务（addr 为空时用 HC_PORT 环境变量或默认端口）
func ListenAndServe(addr string) error {
	if addr == "" {
		port := os.Getenv("HC_PORT")
		if port == "" {
			port = Port
		}
		addr = "0.0.0.0:" + port
	}
	log.Printf("homecast-go listening on http://%s (music + fav + lyric + playlist + cast + speaker + sites + widget)", addr)
	return http.ListenAndServe(addr, New())
}