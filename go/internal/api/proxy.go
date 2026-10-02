package api

import (
	"io"
	"log"
	"net/http"

	"homecast/internal/service"
)

// ProxyHandler token 化流转发（音箱/DLNA 播放用）
type ProxyHandler struct {
	Tokens *service.TokenStore
}

// Video GET /api/v1/proxy/video/{token}
func (h *ProxyHandler) Video(w http.ResponseWriter, r *http.Request) {
	h.stream(w, r, r.PathValue("token"), "video")
}

// Audio GET /api/v1/proxy/audio/{token}
func (h *ProxyHandler) Audio(w http.ResponseWriter, r *http.Request) {
	h.stream(w, r, r.PathValue("token"), "audio")
}

func (h *ProxyHandler) stream(w http.ResponseWriter, r *http.Request, token, kind string) {
	entry, ok := h.Tokens.Get(token)
	if !ok {
		errResp(w, http.StatusNotFound, "token expired or invalid")
		return
	}
	req, err := http.NewRequest(http.MethodGet, entry.URL, nil)
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	if ref := entry.Metadata["referer"]; ref != "" {
		req.Header.Set("Referer", ref)
	}
	if rg := r.Header.Get("Range"); rg != "" {
		req.Header.Set("Range", rg)
	}
	client := &http.Client{ /* 默认跟随重定向 */ }
	upResp, err := client.Do(req)
	if err != nil {
		log.Printf("[proxy %s] upstream: %v", kind, err)
		errResp(w, http.StatusBadGateway, "upstream error")
		return
	}
	defer upResp.Body.Close()

	ct := upResp.Header.Get("Content-Type")
	if ct == "" {
		ct = entry.Metadata["content_type"]
	}
	if ct == "" {
		ct = "video/mp4"
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range")
	w.Header().Set("Content-Type", ct)
	for _, k := range []string{"Content-Length", "Content-Range", "Accept-Ranges"} {
		if v := upResp.Header.Get(k); v != "" {
			w.Header().Set(k, v)
		}
	}
	w.WriteHeader(upResp.StatusCode)
	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, upResp.Body)
	}
}