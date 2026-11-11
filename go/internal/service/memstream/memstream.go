// Package memstream 音箱内存流：整首歌下载到内存 → 按 offset 喂流。
// seek = 改 offset → 音箱重拉从新位置读（手机是唯一状态源，不需音箱 seek API）。
//
// 全链路：SpeakerService.Seek 改 offset + 重下发 URL；api/proxy.go Audio 内存喂流。
package memstream

import (
	"io"
	"log"
	"net/http"
	"sync"

	"homecast/internal/service/token"
)

type stream struct {
	Data   []byte // 整首歌（内存，不落盘）
	Offset int64  // 当前喂流偏移（seek 改这个）
	Mime   string // content-type
}

var (
	streams   = map[string]*stream{} // token -> 流
	mu        sync.Mutex
)

// Get 取流（无则从上游下载整首到内存）
func Get(store *token.TokenStore, tok, kind string) (*stream, error) {
	mu.Lock()
	if s, ok := streams[tok]; ok {
		mu.Unlock()
		return s, nil
	}
	mu.Unlock()

	entry, ok := store.Get(tok)
	if !ok {
		return nil, io.EOF
	}
	req, err := http.NewRequest(http.MethodGet, entry.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	if ref := entry.Metadata["referer"]; ref != "" {
		req.Header.Set("Referer", ref)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = entry.Metadata["content_type"]
	}
	if ct == "" {
		ct = "audio/mpeg"
	}
	log.Printf("[memstream %s] 整首下载到内存: %dB (%s)", kind, len(data), ct)
	s := &stream{Data: data, Offset: 0, Mime: ct}
	mu.Lock()
	streams[tok] = s
	mu.Unlock()
	return s, nil
}

// Seek 改 offset（seek 核心：时间秒 → 字节偏移）
// bitrateKbps=0 用默认 128kbps 估算（VBR 近似；CBR 精确）
func Seek(tok string, sec float64, bitrateKbps int) bool {
	if bitrateKbps <= 0 {
		bitrateKbps = 128
	}
	mu.Lock()
	defer mu.Unlock()
	s, ok := streams[tok]
	if !ok || len(s.Data) == 0 {
		return false
	}
	bytesPerSec := int64(bitrateKbps * 1000 / 8)
	newOffset := int64(float64(bytesPerSec) * sec)
	if newOffset < 0 {
		newOffset = 0
	}
	if newOffset >= int64(len(s.Data)) {
		newOffset = int64(len(s.Data)) - 1
	}
	s.Offset = newOffset
	log.Printf("[memstream seek] token=%s sec=%.1f → offset=%d", tok, sec, newOffset)
	return true
}

// ServeMem 内存喂流（音箱 HTTP 拉流：从当前 offset 读到尾）
func ServeMem(w http.ResponseWriter, r *http.Request, store *token.TokenStore, tok, kind string) {
	s, err := Get(store, tok, kind)
	if err != nil {
		log.Printf("[memstream %s] 取流失败: %v", kind, err)
		http.Error(w, "stream unavailable", http.StatusBadGateway)
		return
	}
	mu.Lock()
	offset := s.Offset
	data := s.Data
	mime := s.Mime
	mu.Unlock()

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Accept-Ranges", "none")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if offset >= int64(len(data)) {
		return
	}
	_, _ = w.Write(data[offset:])
}

// Drop 释放流（停止/切歌时清理内存）
func Drop(tok string) {
	mu.Lock()
	delete(streams, tok)
	mu.Unlock()
}
