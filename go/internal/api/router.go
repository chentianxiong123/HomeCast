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
func NewMux(music *MusicHandler, fav *FavHandler, lyric *LyricHandler, playlist *PlaylistHandler, cast *CastHandler, proxy *ProxyHandler, speaker *SpeakerHandler, sites *SitesHandler) http.Handler {
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
	mux.HandleFunc("GET /api/v1/cast/devices", cast.Devices)
	mux.HandleFunc("POST /api/v1/cast/sniff", cast.Sniff)
	mux.HandleFunc("POST /api/v1/cast/play_url", cast.PlayURL)
	mux.HandleFunc("POST /api/v1/cast/start", cast.Start)
	mux.HandleFunc("POST /api/v1/cast/control", cast.Control)
	mux.HandleFunc("GET /api/v1/cast/status/{device_udn}", cast.Status)
	mux.HandleFunc("GET /api/v1/proxy/video/{token}", proxy.Video)
	mux.HandleFunc("GET /api/v1/proxy/audio/{token}", proxy.Audio)
	mux.HandleFunc("GET /api/v1/speaker/status", speaker.Status)
	mux.HandleFunc("POST /api/v1/speaker/login", speaker.Login)
	mux.HandleFunc("POST /api/v1/speaker/logout", speaker.Logout)
	mux.HandleFunc("GET /api/v1/speaker/devices", speaker.Devices)
	mux.HandleFunc("POST /api/v1/speaker/play", speaker.Play)
	mux.HandleFunc("POST /api/v1/speaker/control", speaker.Control)
	mux.HandleFunc("GET /api/v1/speaker/volume/{did}", speaker.GetVolume)
	mux.HandleFunc("POST /api/v1/speaker/volume", speaker.SetVolume)
	mux.HandleFunc("GET /api/v1/speaker/player_status/{did}", speaker.PlayerStatus)
	mux.HandleFunc("GET /api/v1/speaker/refresh", speaker.Refresh)
	mux.HandleFunc("POST /api/v1/speaker/qr/generate", speaker.QRGenerate)
	mux.HandleFunc("GET /api/v1/speaker/qr/status", speaker.QRStatus)
	mux.HandleFunc("POST /api/v1/speaker/qr/reset", speaker.QRReset)
	mux.HandleFunc("GET /api/v1/sites/list", sites.List)
	mux.HandleFunc("POST /api/v1/sites/add", sites.Add)
	mux.HandleFunc("POST /api/v1/sites/remove", sites.Remove)
	mux.HandleFunc("GET /api/v1/sites/episodes/{detail_url...}", sites.Episodes)
	mux.HandleFunc("POST /api/v1/sites/cache_episodes", sites.CacheEpisodes)
	return mux
}