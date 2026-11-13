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

// StreamGate 音频供给闸门：播放端（音箱）暂停/停止时，其后续拉流请求被断（断源）。
// 统一入口：所有播放端都从 /api/v1/music/stream/{bvid} 拉流；
// 音箱带 &did= 标识自己，暂停时 gate 拒绝该 did → 音箱播完已缓冲的即自然停（音箱 API 无暂停）。
// web 播放器不带 did，不受影响。
type StreamGate interface {
	BlockedStream(did string) bool // did 是否已被暂停/停止（是 → 断源）
}


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
	Gate  StreamGate          // 音频供给闸门（音箱断源用；可空）
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

// serveFile 缓存文件响应（web 播放器 / 音箱 同一入口，支持 Range）。
// http.ServeFile 对 bytes=0- 返回全量（0→末尾），越界返回 416——
// 正是铁证结论：全量一次给完 = 音箱稳定；文件尾探测须 416。
func serveFile(w http.ResponseWriter, r *http.Request, path string) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range")
	http.ServeFile(w, r, path)
}

// Stream GET /api/v1/music/stream/{bvid}?quality=192[&did=音箱设备id]
// 唯一的音频供给入口（web 播放器 / 音箱 / 本地文件 全走这里，源不区分）。
// 缓存优先（秒开）：命中缓存文件直接 serve（web 与音箱同一条 serveFile）；
// 未命中带 referer 直转发 B站流 + 回写缓存。支持 Range（断点续传/拖动进度条）。
// did 非空时受 gate 控制：暂停/停止中 → 断源（音箱拉流失败自然停）。
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

	// 断源闸门：音箱暂停/停止中，后续拉流请求一律拒绝 → 音箱播完缓冲自然停
	did := r.URL.Query().Get("did")
	if did != "" && h.Gate != nil && h.Gate.BlockedStream(did) {
		log.Printf("[stream] 拒绝 %s: did=%s 已暂停/停止（断源）", bvid, did)
		errResp(w, http.StatusServiceUnavailable, "stream suspended")
		return
	}

	// 1. 缓存命中：serve 本地文件（web 与音箱同一条 serveFile，不分叉。
	//    全量 200/全量 Range 一次给 = 音箱铁证稳定；206 分片喂流已证伪删除）
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

	// 3. 回写缓存：仅全量 200 响应（非 Range/206）才 tee 落盘，且 StreamAndCache 内部校验
	// 字节数 == 上游 Content-Length 才落盘（下载中断的截断文件直接丢弃，不污染缓存）
	if h.Cache != nil && h.Cache.LimitMB() > 0 && r.Header.Get("Range") == "" && upResp.StatusCode == http.StatusOK {
		h.Cache.StreamAndCache(w, bvid, quality, upResp.Body, upResp.ContentLength)
	} else {
		_, _ = io.Copy(w, upResp.Body)
	}
}
