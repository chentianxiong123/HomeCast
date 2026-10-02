package api

import (
	"encoding/json"
	"log"
	"net/http"

	"homecast/internal/service"
)

// CastHandler 投屏端点
type CastHandler struct {
	Cast *service.CastService
}

// Devices GET /api/v1/cast/devices
func (h *CastHandler) Devices(w http.ResponseWriter, r *http.Request) {
	h.Cast.Discover()
	ok(w, h.Cast.DeviceList())
}

// Sniff POST /api/v1/cast/sniff  body: {url}
func (h *CastHandler) Sniff(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		errResp(w, http.StatusBadRequest, "url required")
		return
	}
	ok(w, h.Cast.Sniff(body.URL))
}

// PlayURL POST /api/v1/cast/play_url  body: {url, title}
func (h *CastHandler) PlayURL(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL   string `json:"url"`
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		errResp(w, http.StatusBadRequest, "url required")
		return
	}
	res, _ := h.Cast.PlayURL(body.URL, body.Title)
	writeJSON(w, http.StatusOK, res)
}

// Start POST /api/v1/cast/start  body: {episode_url, device_udn, title}
func (h *CastHandler) Start(w http.ResponseWriter, r *http.Request) {
	var body struct {
		EpisodeURL string `json:"episode_url"`
		DeviceUDN  string `json:"device_udn"`
		Title      string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.DeviceUDN == "" {
		errResp(w, http.StatusBadRequest, "device_udn required")
		return
	}
	res, err := h.Cast.Cast(body.EpisodeURL, body.DeviceUDN, body.Title)
	if err != nil {
		log.Printf("[cast] %v", err)
	}
	writeJSON(w, http.StatusOK, res)
}

// Control POST /api/v1/cast/control  body: {device_udn, action, target, volume}
func (h *CastHandler) Control(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DeviceUDN string `json:"device_udn"`
		Action    string `json:"action"`
		Target    string `json:"target"`
		Volume    int    `json:"volume"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.DeviceUDN == "" {
		errResp(w, http.StatusBadRequest, "device_udn required")
		return
	}
	res, err := h.Cast.Control(body.DeviceUDN, body.Action, body.Target, body.Volume)
	if err != nil {
		log.Printf("[cast control] %v", err)
	}
	writeJSON(w, http.StatusOK, res)
}

// Status GET /api/v1/cast/status/{device_udn}
func (h *CastHandler) Status(w http.ResponseWriter, r *http.Request) {
	res, err := h.Cast.Status(r.PathValue("device_udn"))
	if err != nil {
		log.Printf("[cast status] %v", err)
	}
	writeJSON(w, http.StatusOK, res)
}