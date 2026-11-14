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
	"time"

	"homecast/internal/bilibili"
	"homecast/internal/service/cache"
	"homecast/internal/service/fmp4"
	"homecast/internal/service/music"
	"homecast/internal/service/transcode"
)

// StreamGate 音频供给闸门：播放端（音箱）暂停/停止时，其后续拉流请求被断（断源）。
// 统一入口：所有播放端都从 /api/v1/music/stream/{bvid} 拉流；
// 音箱带 &did= 标识自己，暂停时 gate 拒绝该 did → 音箱播完已缓冲的即自然停（音箱 API 无暂停）。
// web 播放器不带 did，不受影响。
type StreamGate interface {
	BlockedStream(did string) bool // did 是否已被暂停/停止（是 → 断源）
	// AnchorStream 音箱首次拉流校准：命令下发到音箱真正拉流有延迟，进度基准
	// 应从拉流时刻起算（AnchorPos=拉流瞬间应处进度），否则 dock 进度虚增那段延迟
	AnchorStream(did string, at time.Time)
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
	Music  *music.MusicService
	Cache  *cache.CacheService    // 音频缓存（可空：无缓存功能时直转发）
	Gate   StreamGate             // 音频供给闸门（音箱断源用；可空）
	Xcoder *transcode.Transcoder  // 音箱软转码 mp3 秒开（可空/ffmpeg 缺失 → 音箱回退原 m4s 路径）
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
	// 拉流校准：命令下发到音箱真正拉流有延迟，音箱拉流时刻为进度真实起点。
	// 每次拉流都上报（锚定逻辑内部只在首次/续播后生效，重复拉流幂等）
	if did != "" && h.Gate != nil {
		h.Gate.AnchorStream(did, time.Now())
	}

	// 续播/跳播秒：音箱端 resume 时带 start=暂停位置（fMP4 容器级 seek 切流，从中间开始）
	startSec := 0.0
	if v := r.URL.Query().Get("start"); v != "" {
		startSec, _ = strconv.ParseFloat(v, 64)
	}

	// ★ 音箱秒开分支：喂转码 mp3（fMP4 容器是三四秒出声根因，mp3 流式帧拿到即出声）。
	// 仅音箱（did 非空）走此路径；web 播放器（did 空）保持原 m4s 路径（浏览器原生解 fMP4，
	// 不需转码且保留 fMP4 容器级 seek）。Xcoder 不可用（ffmpeg 缺失/未注入）→ 回退原 m4s 路径。
	if did != "" && h.Xcoder != nil && h.Xcoder.Available() && h.Cache != nil {
		h.streamSpeakerMP3(w, r, bvid, quality, startSec)
		return
	}

	// 1. 缓存命中：serve 本地文件（web 与音箱同一条 serveFile，不分叉。
	//    全量 200/全量 Range 一次给 = 音箱铁证稳定；206 分片喂流已证伪删除）
	if h.Cache != nil {
		if _, _, ok := h.Cache.Get(bvid); ok {
			if startSec > 0 {
				// 从中间开始：fMP4 sample 级 seek 切流（纯容器操作不转码）。
				// 解析失败/非 fMP4 一律回退全量从头播（绝不给音箱喂非法字节）
				if out, err := fmp4.Seek(h.Cache.FilePath(bvid), startSec); err == nil && len(out) > 0 {
					w.Header().Set("Access-Control-Allow-Origin", "*")
					w.Header().Set("Content-Type", "application/octet-stream")
					w.Header().Set("Content-Length", fmt.Sprint(len(out)))
					w.WriteHeader(http.StatusOK)
					if r.Method != http.MethodHead {
						_, _ = w.Write(out)
					}
					log.Printf("[stream] %s start=%.1fs 切流 %dB (源 %s)", bvid, startSec, len(out), h.Cache.FilePath(bvid))
					return
				}
				log.Printf("[stream] %s start=%.1fs 切流失败 → 回退全量", bvid, startSec)
			}
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

// streamSpeakerMP3 音箱秒开供给：缓存 mp3 命中直接 serve；未命中取源 m4s → 转码 mp3
// （转完落缓存）→ serve。续播用字节 seek（mp3 无容器级 seek，CBR 线性换算 + 帧对齐）。
// 全程 200 全量续流（踩"200 全量稳定 / 206 分片炸音"铁证；音箱是哑执行器，给它一段
// 从指定字节起的完整剩余 mp3，不等它 Range 自己 seek）。
func (h *MusicHandler) streamSpeakerMP3(w http.ResponseWriter, r *http.Request, bvid string, quality int, startSec float64) {
	// 1. mp3 缓存命中 → 直接 seek serve（同曲二次秒开）
	if _, ok := h.Xcoder.Get(bvid, quality); ok {
		h.serveMP3Seek(w, r, h.Xcoder.FilePath(bvid, quality), startSec)
		return
	}
	// 2. 确保源 m4s 落盘（mp3 从 m4s 派生；m4s 复用现有缓存，不重复下载）
	m4sPath, err := h.ensureM4S(bvid, quality)
	if err != nil {
		log.Printf("[stream] 音箱转码取源失败 %s: %v", bvid, err)
		errResp(w, http.StatusInternalServerError, "source error: "+err.Error())
		return
	}
	// 3. 转码 mp3（转完落 mp3 缓存；同步转整首，纯 CPU 几十倍实时，几秒）
	mp3Path, err := h.Xcoder.EnsureMP3(m4sPath, bvid, quality)
	if err != nil {
		log.Printf("[stream] 音箱转码失败 %s: %v", bvid, err)
		errResp(w, http.StatusInternalServerError, "transcode error: "+err.Error())
		return
	}
	// 4. seek serve mp3
	h.serveMP3Seek(w, r, mp3Path, startSec)
}

// ensureM4S 确保源 m4s 已落缓存并返回路径。命中直接返回；未命中拉 B站全量落盘
// （复用现有 Cache.Put，不改 cache 包）。音箱转码需要本地 m4s 文件作 ffmpeg 输入。
func (h *MusicHandler) ensureM4S(bvid string, quality int) (string, error) {
	if _, _, ok := h.Cache.Get(bvid); ok {
		return h.Cache.FilePath(bvid), nil
	}
	stream, err := h.Music.GetAudioStream(bvid, quality)
	if err != nil {
		return "", err
	}
	if stream.URL == "" {
		return "", fmt.Errorf("empty audio URL")
	}
	upResp, err := h.Music.Client.GetRaw(stream.URL, nil)
	if err != nil {
		return "", err
	}
	defer upResp.Body.Close()
	if upResp.StatusCode != http.StatusOK && upResp.StatusCode != http.StatusPartialContent {
		return "", fmt.Errorf("upstream %d", upResp.StatusCode)
	}
	data, err := io.ReadAll(upResp.Body)
	if err != nil {
		return "", err
	}
	// 完整性校验（对齐现有缓存纪律）：声明长度却下不满 → 截断，丢弃不污染缓存
	if upResp.ContentLength > 0 && int64(len(data)) != upResp.ContentLength {
		return "", fmt.Errorf("truncated %d/%d", len(data), upResp.ContentLength)
	}
	if err := h.Cache.Put(bvid, quality, data); err != nil {
		return "", err
	}
	return h.Cache.FilePath(bvid), nil
}

// serveMP3Seek 从 mp3 指定字节起 serve 完整剩余段（200 全量 + Content-Length=剩余）。
// 起始字节由 SeekOffset 按 CBR 线性换算（秒→字节 + 帧对齐）；越界回退从头。
// 不处理 Range（音箱不自己 seek，全量续流稳定）。
func (h *MusicHandler) serveMP3Seek(w http.ResponseWriter, r *http.Request, mp3Path string, startSec float64) {
	fi, err := os.Stat(mp3Path)
	if err != nil || fi.Size() == 0 {
		errResp(w, http.StatusInternalServerError, "mp3 missing")
		return
	}
	offset := h.Xcoder.SeekOffset(startSec)
	if offset >= fi.Size() {
		offset = 0
	}
	f, err := os.Open(mp3Path)
	if err != nil {
		errResp(w, http.StatusInternalServerError, "mp3 open")
		return
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return
	}
	// ServeContent 原生处理 Range（206 + Content-Range + Accept-Ranges: bytes）。
	// 关键：音箱会发 Range: bytes=N- 分段续拉（拉一段缓冲够就停，播完回来续拉）。
	// 旧实现无视 Range 回 200 全量 → 打乱音箱缓冲节奏 → 多次重拉 = 出声延迟。
	// 正确 206 续拉 → 音箱要哪段给哪段，缓冲够即出声（实测 ~1s）。
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "audio/mpeg")
	http.ServeContent(w, r, "", time.Now(), f)
	log.Printf("[stream] serveMP3 %s start=%.1fs offset=%d size=%d Range=%q", mp3Path, startSec, offset, fi.Size(), r.Header.Get("Range"))
}
