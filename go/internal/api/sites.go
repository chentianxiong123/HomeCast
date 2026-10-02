package api

import (
	"encoding/json"
	"net/http"
	"net/url"

	"homecast/internal/service"
)

// SitesHandler 站点管理端点
type SitesHandler struct {
	Sites *service.SitesService
}

// List GET /api/v1/sites/list
func (h *SitesHandler) List(w http.ResponseWriter, r *http.Request) {
	ok(w, h.Sites.List())
}

// Add POST /api/v1/sites/add  body: {name, url, site_type}
func (h *SitesHandler) Add(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		URL      string `json:"url"`
		SiteType string `json:"site_type"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		errResp(w, http.StatusBadRequest, "url required")
		return
	}
	if body.SiteType == "" {
		body.SiteType = "video"
	}
	if err := h.Sites.Add(service.Site{Name: body.Name, URL: body.URL, SiteType: body.SiteType}); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "message": err.Error(), "data": nil})
		return
	}
	ok(w, true)
}

// Remove POST /api/v1/sites/remove  body: {url}
func (h *SitesHandler) Remove(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		errResp(w, http.StatusBadRequest, "url required")
		return
	}
	h.Sites.Remove(body.URL)
	ok(w, true)
}

// Episodes GET /api/v1/sites/episodes/{detail_url:path}
func (h *SitesHandler) Episodes(w http.ResponseWriter, r *http.Request) {
	detailURL, err := url.PathUnescape(r.PathValue("detail_url"))
	if err != nil {
		detailURL = r.PathValue("detail_url")
	}
	if cached, ok := h.Sites.GetCachedEpisodes(detailURL); ok {
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": cached, "cached": true})
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]any{"code": 404, "message": "缓存未找到"})
}

// CacheEpisodes POST /api/v1/sites/cache_episodes  body: {detail_url, title, episodes_list}
func (h *SitesHandler) CacheEpisodes(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DetailURL    string            `json:"detail_url"`
		Title        string            `json:"title"`
		EpisodesList []service.Episode `json:"episodes_list"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.DetailURL == "" {
		errResp(w, http.StatusBadRequest, "detail_url required")
		return
	}
	h.Sites.CacheEpisodes(body.DetailURL, body.Title, body.EpisodesList)
	ok(w, true)
}