// Package api HTTP 端点（对齐 Python /api/v1 前缀）
package api

import (
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"homecast/internal/bilibili"
	"homecast/internal/service/cache"
	"homecast/internal/service/music"
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
	Music *music.MusicService
	Cache *cache.CacheService // 音频缓存（可空：无缓存功能时直转发）
}

// NewMusicHandler 构造
func NewMusicHandler(m *music.MusicService) *MusicHandler {
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

// serveFile 本地缓存文件响应（支持 Range：拖动进度条）
func serveFile(w http.ResponseWriter, r *http.Request, path string) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range")
	http.ServeFile(w, r, path)
}

// Stream GET /api/v1/music/stream/{bvid}?quality=192
// 缓存优先（秒开）：命中缓存文件直接 serve；未命中带 referer 直转发 B站流。
// 支持 Range（播放器拖动进度条）。
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

	// 1. 缓存命中：直接 serve 本地文件（秒开，支持 Range）
	if h.Cache != nil {
		if _, _, ok := h.Cache.Get(bvid); ok {
			serveFile(w, r, h.Cache.FilePath(bvid))
			return
		}
	}

	// 2. 未命中：取 B站流地址并转发
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
	if r.Method == http.MethodHead {
		return
	}

	// 3. 回写缓存：上限开启且非 Range（全量流）时，边转发边 tee 落盘
	if h.Cache != nil && h.Cache.LimitMB() > 0 && r.Header.Get("Range") == "" {
		h.Cache.StreamAndCache(w, bvid, quality, upResp.Body)
	} else {
		_, _ = io.Copy(w, upResp.Body)
	}
}
