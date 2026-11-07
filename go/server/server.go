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
	"homecast/internal/service/cast"
	"homecast/internal/service/fav"
	"homecast/internal/service/lyric"
	"homecast/internal/service/music"
	"homecast/internal/service/playlist"
	"homecast/internal/service/sites"
	spk "homecast/internal/service/speaker"
	"homecast/internal/service/token"
	"homecast/internal/service/widget"
	"homecast/internal/speaker"
	"homecast/internal/store"
	"homecast/web"
)

// Port 默认端口：Go 版独立端口，与 Python 28974 并存对照
const Port = "28976"

// New 组装全部 handler（带 CORS，桌面壳跨域用）
// E2E/测试注入：环境变量 HC_BILI_BASE（假 B站 base URL，默认 api.bilibili.com）、
// HC_DATA_DIR（数据目录，默认 ~/.config/homecast）——走真实 HTTP 全链路但不依赖外网、不污染用户数据。
func New() http.Handler {
	biliBase := os.Getenv("HC_BILI_BASE")
	if biliBase == "" {
		biliBase = "https://api.bilibili.com"
	}
	client := bilibili.NewClient(
		biliBase,
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"https://search.bilibili.com/", // 对齐 Python 默认值！www.bilibili.com 会被风控返回 HTML
		30*time.Second,
	)

	// SQLite 单文件持久层：首启自动从旧 JSON 迁移（JSON 保留只读备份）
	dataDir := os.Getenv("HC_DATA_DIR")
	if dataDir == "" {
		home, _ := os.UserHomeDir()
		dataDir = filepath.Join(home, ".config", "homecast")
	}
	db, err := store.Open(filepath.Join(dataDir, "homecast.db"))
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	if err := store.Migrate(db, []string{
		fav.FavSchema,
		playlist.PlaylistSchema,
		lyric.LyricSchema,
	}, map[string]string{
		fav.FavTable:     filepath.Join(dataDir, "favorites.json"),
		playlist.PlaylistTable: filepath.Join(dataDir, "playlist.json"),
	}); err != nil {
		log.Fatalf("migrate db: %v", err)
	}

	musicSvc := &music.MusicService{Client: client}
	musicHandler := api.NewMusicHandler(musicSvc)
	favHandler := &api.FavHandler{Fav: fav.NewFavService(db)}
	lyricHandler := &api.LyricHandler{Lyric: lyric.NewLyricStore(db), Music: musicSvc}
	// offset 变更 → 挂件重定位广播（闭包引用 widgetStateGlobal，server 建完再赋值）
	lyricHandler.OnOffsetChange = func() {
		if widgetStateGlobal != nil {
			widgetStateGlobal.InvalidateLines() // 清缓存，下次 currentLine 重拉用新 offset
			widgetStateGlobal.BroadcastLyricState()
		}
	}
	// 选源变更 → 清缓存重拉 + SSE 广播（歌词页点候选 / 挂件同源）
	lyricHandler.OnSelectChange = func() {
		if widgetStateGlobal != nil {
			widgetStateGlobal.InvalidateLines()
			widgetStateGlobal.BroadcastLyricState()
		}
	}
	playlistHandler := &api.PlaylistHandler{Playlist: playlist.NewPlaylistService(client, db)}
	tokenStore := token.NewTokenStore()
	castSvc := cast.NewCastService(client, tokenStore)
	castHandler := &api.CastHandler{Cast: castSvc}
	proxyHandler := &api.ProxyHandler{Tokens: tokenStore}
	speakerSvc := spk.NewSpeakerService(speaker.NewSpeakerAuth(), client, tokenStore)
	speakerQR := speaker.NewQRLogin()
	speakerHandler := &api.SpeakerHandler{Svc: speakerSvc, QR: speakerQR}
	sitesHandler := &api.SitesHandler{Sites: sites.NewSitesService("")}
	widgetState := widget.NewWidgetState() // 先建 state（source 需引用 state 取 title）
	lyricSrc := newLyricSource(lyricHandler, widgetState) // source 引用 handler+state
	widgetState.SetLineSrc(lyricSrc)                       // 注入 source（避免循环依赖）
	widgetStateGlobal = widgetState
	widgetHandler := &api.WidgetHandler{State: widgetState}

	mux := api.NewMux(
		musicHandler, favHandler, lyricHandler, playlistHandler,
		castHandler, proxyHandler, speakerHandler, sitesHandler, widgetHandler,
	)

	// htmx 层（Go 渲染页面 + 功能切片）：/ 首页、/assets 静态、/hx/* 切片
	hxH := &hx.H{Music: musicSvc, Fav: fav.NewFavService(db), Cast: castSvc, Speaker: speakerSvc, QR: speakerQR, Tpl: web.MustTemplates()}
	outer := http.NewServeMux()
	outer.Handle("/", hxH.Router())
	outer.Handle("/api/", mux)
	return withCORS(outer)
}

// WidgetStateRef 挂件（同进程）读取播放状态用
func WidgetStateRef() *widget.WidgetState { return widgetStateGlobal }

// WidgetStateSnapshot 公开快照（desktop/widget 轮询用，不暴露内部类型）
func WidgetStateSnapshot() (bvid, title, artist string, currentTime, duration float64, playing bool) {
	if widgetStateGlobal != nil {
		return widgetStateGlobal.Get()
	}
	return "", "", "", 0, 0, false
}

var widgetStateGlobal *widget.WidgetState

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