package api

import (
	"encoding/json"
	"net/http"

	"homecast/internal/service"
)

// FavHandler 收藏夹端点（GET/POST/DELETE /api/v1/fav/*）
type FavHandler struct {
	Fav *service.FavService
}

// List GET /api/v1/fav/list
func (h *FavHandler) List(w http.ResponseWriter, r *http.Request) {
	ok(w, h.Fav.List())
}

// Add POST /api/v1/fav/add  body: FavSong
func (h *FavHandler) Add(w http.ResponseWriter, r *http.Request) {
	var e service.FavSong
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil || e.BVID == "" {
		errResp(w, http.StatusBadRequest, "invalid body: bvid required")
		return
	}
	ok(w, h.Fav.Add(e))
}

// Remove DELETE /api/v1/fav/{bvid}
func (h *FavHandler) Remove(w http.ResponseWriter, r *http.Request) {
	bvid := r.PathValue("bvid")
	if bvid == "" {
		errResp(w, http.StatusBadRequest, "bvid required")
		return
	}
	ok(w, h.Fav.Remove(bvid))
}

// Clear POST /api/v1/fav/clear
func (h *FavHandler) Clear(w http.ResponseWriter, r *http.Request) {
	ok(w, h.Fav.Clear())
}