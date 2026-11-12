// Package api HTTP 端点（对齐 Python /api/v1 前缀）
package api

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
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

// serveFile 本地缓存文件响应（支持 Range：拖动进度条）
func serveFile(w http.ResponseWriter, r *http.Request, path string) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range")
	http.ServeFile(w, r, path)
}

// chunkSize 音箱分片喂流片长：0.25s @192k ≈ 6KB（11-10 二分实测最优）。
// 每个 Range 请求只回这一小片就断连 → 音箱解码器永远只缓冲 0.25s →
// 暂停/切歌立即生效（不再等 877KB 预缓冲播完 20s+）。
const chunkSize = 6 * 1024

// serveSpeakerChunk 音箱分片喂流：解析 Range（无则从头），每次只回 6KB 一片（206 + Content-Range），
// 写完即返回断连。音箱播完这片自带下次 Range 请求 → 缓冲常小。ct = 内容类型。
func serveSpeakerChunk(w http.ResponseWriter, r *http.Request, path string, ct string) {
	f, err := os.Open(path)
	if err != nil {
		errResp(w, http.StatusInternalServerError, "open cache: "+err.Error())
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		errResp(w, http.StatusInternalServerError, "stat: "+err.Error())
		return
	}
	size := st.Size()
	if size <= 0 {
		errResp(w, http.StatusNotFound, "empty cache file")
		return
	}

	start := int64(0)
	if n, ok := rangeStart(r.Header.Get("Range")); ok {
		start = n
	}
	if start >= size {
		start = 0 // 超出末尾回开头（循环/重放场景）
	}
	end := start + chunkSize
	if end > size {
		end = size
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end-1, size))
	w.Header().Set("Content-Length", strconv.FormatInt(end-start, 10))
	w.WriteHeader(http.StatusPartialContent) // 206：只回一片
	if r.Method == http.MethodHead {
		return
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return
	}
	_, _ = io.CopyN(w, f, end-start)
}

// rangeStart 解析单段 Range 头（bytes=N- / bytes=N-M），返回起始字节（无/解析失败 → false）。
func rangeStart(rg string) (int64, bool) {
	const p = "bytes="
	if !strings.HasPrefix(rg, p) {
		return 0, false
	}
	rest := strings.TrimSpace(rg[len(p):])
	if rest == "" || strings.Contains(rest, ",") {
		return 0, false
	}
	dash := strings.IndexByte(rest, '-')
	if dash <= 0 {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.TrimSpace(rest[:dash]), 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// Stream GET /api/v1/music/stream/{bvid}?quality=192[&did=音箱设备id]
// 唯一的音频供给入口（web 播放器 / 音箱 / 未来本地文件 全走这里，源不区分）。
// 缓存优先（秒开）：命中缓存文件直接 serve；未命中带 referer 直转发 B站流 + 回写缓存。
// 支持 Range（断点续传/拖动进度条）。
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

	// 1. 缓存命中：serve 本地文件
	//    音箱（did 非空）→ 分片喂流（每次 0.25s 一片，缓冲常小 → 暂停/切歌跟手）
	//    web → 完整 serveFile（浏览器自己控制缓冲/拖动）
	if h.Cache != nil {
		if _, _, ok := h.Cache.Get(bvid); ok {
			if did != "" {
				serveSpeakerChunk(w, r, h.Cache.FilePath(bvid), "audio/mpeg")
			} else {
				serveFile(w, r, h.Cache.FilePath(bvid))
			}
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
