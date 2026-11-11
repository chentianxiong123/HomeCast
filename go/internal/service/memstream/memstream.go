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
	Bitrate int   // 真实平均码率 kbps（seek 时间→字节用；从 token metadata "bitrate" 读）
}

var (
	streams   = map[string]*stream{} // token -> 流
	mu        sync.Mutex
)

// Get 取流（data 非空用之【缓存文件预载】；否则从上游下载整首到内存）。
// 真实码率从 token metadata "bitrate" 读（缓存=Size×8/时长，本体流=DASH Bandwidth）。
func Get(store *token.TokenStore, tok, kind string, data []byte) (*stream, error) {
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
	var body []byte
	ct := entry.Metadata["content_type"]
	if data != nil {
		body = data // 缓存文件预载（不重下）
	} else {
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
		d, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		body = d
		if rct := resp.Header.Get("Content-Type"); rct != "" {
			ct = rct
		}
	}
	if ct == "" {
		ct = "audio/mpeg"
	}
	bitrate := atoiDefault(entry.Metadata["bitrate"], 128)
	log.Printf("[memstream %s] 整首进内存: %dB (%s) 码率=%dkbps", kind, len(body), ct, bitrate)
	s := &stream{Data: body, Offset: 0, Mime: ct, Bitrate: bitrate}
	mu.Lock()
	streams[tok] = s
	mu.Unlock()
	return s, nil
}

// atoiDefault 解析字符串整数（失败/空返 def）
func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return def
		}
		v = v*10 + int(c-'0')
	}
	return v
}

// Seek 改 offset（seek 核心：时间秒 → 字节偏移，用流的真实码率）
func Seek(tok string, sec float64) bool {
	mu.Lock()
	defer mu.Unlock()
	s, ok := streams[tok]
	if !ok || len(s.Data) == 0 {
		return false
	}
	bitrate := s.Bitrate
	if bitrate <= 0 {
		bitrate = 128
	}
	bytesPerSec := int64(bitrate * 1000 / 8)
	newOffset := int64(float64(bytesPerSec) * sec)
	if newOffset < 0 {
		newOffset = 0
	}
	if newOffset >= int64(len(s.Data)) {
		newOffset = int64(len(s.Data)) - 1
	}
	s.Offset = newOffset
	log.Printf("[memstream seek] token=%s sec=%.1f 码率=%dkbps → offset=%d/%d", tok, sec, bitrate, newOffset, len(s.Data))
	return true
}

// ServeMem 内存喂流（音箱 HTTP 拉流：从当前 offset 读到尾）
func ServeMem(w http.ResponseWriter, r *http.Request, store *token.TokenStore, tok, kind string) {
	s, err := Get(store, tok, kind, nil) // 喂流时流已建（Play/Seek 建好）；无则现下（码率从 metadata 读）
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
