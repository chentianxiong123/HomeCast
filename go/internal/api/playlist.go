package api

import (
	"log"
	"net/http"

	"homecast/internal/service"
)

// PlaylistHandler 播放列表端点
type PlaylistHandler struct {
	Playlist *service.PlaylistService
}

// Get GET /api/v1/playlist
func (h *PlaylistHandler) Get(w http.ResponseWriter, r *http.Request) {
	ok(w, h.Playlist.Get())
}

// Add POST /api/v1/playlist/add/{bvid}
func (h *PlaylistHandler) Add(w http.ResponseWriter, r *http.Request) {
	bvid := r.PathValue("bvid")
	if bvid == "" {
		errResp(w, http.StatusBadRequest, "bvid required")
		return
	}
	item, err := h.Playlist.Add(bvid)
	if err != nil {
		log.Printf("[playlist add] %s: %v", bvid, err)
		errResp(w, http.StatusInternalServerError, "failed to add song")
		return
	}
	ok(w, item)
}

// Remove POST /api/v1/playlist/remove/{bvid}
func (h *PlaylistHandler) Remove(w http.ResponseWriter, r *http.Request) {
	bvid := r.PathValue("bvid")
	success := h.Playlist.Remove(bvid)
	if !success {
		errResp(w, http.StatusNotFound, "not found")
		return
	}
	ok(w, true)
}

// Clear POST /api/v1/playlist/clear
func (h *PlaylistHandler) Clear(w http.ResponseWriter, r *http.Request) {
	h.Playlist.Clear()
	ok(w, true)
}