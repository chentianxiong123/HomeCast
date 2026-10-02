// Package server 组装 homecast 全部后端 handler（HTTP 服务）
// 被 cmd/server（独立进程）和 desktop（Wails 壳内嵌）复用
package server

import (
	"log"
	"net/http"
	"os"
	"time"

	"homecast/internal/api"
	"homecast/internal/bilibili"
	"homecast/internal/service"
	"homecast/internal/speaker"
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

	musicSvc := &service.MusicService{Client: client}
	musicHandler := api.NewMusicHandler(musicSvc)
	favHandler := &api.FavHandler{Fav: service.NewFavService("")}
	lyricHandler := &api.LyricHandler{}
	playlistHandler := &api.PlaylistHandler{Playlist: service.NewPlaylistService(client, "")}
	tokenStore := service.NewTokenStore()
	castHandler := &api.CastHandler{Cast: service.NewCastService(client, tokenStore)}
	proxyHandler := &api.ProxyHandler{Tokens: tokenStore}
	speakerSvc := service.NewSpeakerService(speaker.NewSpeakerAuth(), client, tokenStore)
	speakerHandler := &api.SpeakerHandler{Svc: speakerSvc, QR: speaker.NewQRLogin()}
	sitesHandler := &api.SitesHandler{Sites: service.NewSitesService("")}

	return withCORS(api.NewMux(
		musicHandler, favHandler, lyricHandler, playlistHandler,
		castHandler, proxyHandler, speakerHandler, sitesHandler,
	))
}

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
	log.Printf("homecast-go listening on http://%s (music + fav + lyric + playlist + cast + speaker + sites)", addr)
	return http.ListenAndServe(addr, New())
}