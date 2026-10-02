package api

import (
	"encoding/json"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// NewMux 组装全部路由（对齐 Python /api/v1 前缀）
func NewMux(music *MusicHandler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/music/search", music.Search)
	mux.HandleFunc("GET /api/v1/music/stream/{bvid}", music.Stream)
	return mux
}
