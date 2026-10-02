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
func NewMux(music *MusicHandler, fav *FavHandler, lyric *LyricHandler, playlist *PlaylistHandler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/music/search", music.Search)
	mux.HandleFunc("GET /api/v1/music/stream/{bvid}", music.Stream)
	mux.HandleFunc("GET /api/v1/music/lyric", lyric.Get)
	mux.HandleFunc("GET /api/v1/music/lyric/candidates", lyric.Candidates)
	mux.HandleFunc("GET /api/v1/fav/list", fav.List)
	mux.HandleFunc("POST /api/v1/fav/add", fav.Add)
	mux.HandleFunc("DELETE /api/v1/fav/{bvid}", fav.Remove)
	mux.HandleFunc("POST /api/v1/fav/clear", fav.Clear)
	mux.HandleFunc("GET /api/v1/playlist", playlist.Get)
	mux.HandleFunc("POST /api/v1/playlist/add/{bvid}", playlist.Add)
	mux.HandleFunc("POST /api/v1/playlist/remove/{bvid}", playlist.Remove)
	mux.HandleFunc("POST /api/v1/playlist/clear", playlist.Clear)
	return mux
}