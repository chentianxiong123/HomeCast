// Package api HTTP 端点（对齐 Python /api/v1 前缀）
package api

import (
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"homecast/internal/bilibili"
	"homecast/internal/service"
)

// ok 统一响应 {code:0,message:"success",data:...}（前端拦截器依赖 code==0）
func ok(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, map[string]any{
		"code": 0, "message": "success", "data": data,
	})
}

func errResp(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{
		"code": status, "message": msg, "data": nil,
	})
}

// MusicHandler 音乐端点集合
type MusicHandler struct {
	Music *service.MusicService
}

// NewMusicHandler 构造
func NewMusicHandler(m *service.MusicService) *MusicHandler {
	return &MusicHandler{Music: m}
}

// Search GET /api/v1/music/search?keyword=&page=&page_size=
func (h *MusicHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	keyword := strings.TrimSpace(q.Get("keyword"))
	if keyword == "" {
		errResp(w, http.StatusBadRequest, "keyword required")
		return
	}
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}
	res, err := h.Music.Search(keyword, page, pageSize)
	if err != nil {
		log.Printf("[search] %s: %v", keyword, err)
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	ok(w, res)
}

// Stream GET /api/v1/music/stream/{bvid}?quality=192
// DASH 音频直转：带 referer 头直接转发流，不转码不缓存，支持 Range。
func (h *MusicHandler) Stream(w http.ResponseWriter, r *http.Request) {
	bvid := r.PathValue("bvid")
	if bvid == "" {
		errResp(w, http.StatusBadRequest, "bvid required")
		return
	}
	quality, _ := strconv.Atoi(r.URL.Query().Get("quality"))
	if quality == 0 {
		quality = bilibili.PreferredQuality
	}
	stream, err := h.Music.GetAudioStream(bvid, quality)
	if err != nil {
		log.Printf("[stream] %s: %v", bvid, err)
		errResp(w, http.StatusInternalServerError, "Failed to get audio: "+err.Error())
		return
	}
	if stream.URL == "" {
		errResp(w, http.StatusInternalServerError, "Empty audio URL")
		return
	}

	// 带 referer 直转发 B站流
	extra := http.Header{}
	if rg := r.Header.Get("Range"); rg != "" {
		extra.Set("Range", rg)
	}
	upResp, err := h.Music.Client.GetRaw(stream.URL, extra)
	if err != nil {
		log.Printf("[stream] upstream %s: %v", bvid, err)
		errResp(w, http.StatusBadGateway, "upstream error")
		return
	}
	defer upResp.Body.Close()

	// 透传上游状态/头（content-type、content-length、content-range、accept-ranges）
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range")
	for _, k := range []string{
		"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges",
	} {
		if v := upResp.Header.Get(k); v != "" {
			w.Header().Set(k, v)
		}
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "audio/mpeg")
	}
	w.WriteHeader(upResp.StatusCode)
	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, upResp.Body)
	}
}
