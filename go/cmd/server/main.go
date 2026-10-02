// homecast Go 后端入口（阶段1：搜索 + DASH 音频流）
package main

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

func main() {
	port := os.Getenv("HC_PORT")
	if port == "" {
		port = "28976" // Go 版独立端口，与 Python 28974 并存对照
	}

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

	addr := "0.0.0.0:" + port
	log.Printf("homecast-go listening on http://%s (music + fav + lyric + playlist + cast + speaker)", addr)
	if err := http.ListenAndServe(addr, api.NewMux(musicHandler, favHandler, lyricHandler, playlistHandler, castHandler, proxyHandler, speakerHandler)); err != nil {
		log.Fatal(err)
	}
}
