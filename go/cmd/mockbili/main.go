// mockbili 假 B站 API server：供 E2E 无外网跑（前端/后端全链路）
// 用法：HC_BILI_BASE=http://127.0.0.1:39001 go run ./cmd/mockbili  &（起 39001）
// 提供：/x/web-interface/search/type（搜索结果）/x/web-interface/view（cid）
//       /x/player/playurl（DASH 音频 baseUrl 指回自己）/audio.m4s（静音 WAV，真实可播）
package main

import (
	"encoding/binary"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
)

const port = ":39001"

// wavSilence 生成 1 秒 16-bit 44100Hz 单声道静音 WAV（前端 <audio> 真实可播，audio.playing=true）
func wavSilence() []byte {
	sampleRate := 44100
	samples := sampleRate // 1 秒
	dataSize := samples * 2
	buf := make([]byte, 44+dataSize)
	// RIFF header
	copy(buf[0:], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:], uint32(36+dataSize))
	copy(buf[8:], "WAVE")
	copy(buf[12:], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:], 16)
	binary.LittleEndian.PutUint16(buf[20:], 1)          // PCM
	binary.LittleEndian.PutUint16(buf[22:], 1)          // mono
	binary.LittleEndian.PutUint32(buf[24:], uint32(sampleRate))
	binary.LittleEndian.PutUint32(buf[28:], uint32(sampleRate*2)) // byte rate
	binary.LittleEndian.PutUint16(buf[32:], 2)          // block align
	binary.LittleEndian.PutUint16(buf[34:], 16)         // bits
	copy(buf[36:], "data")
	binary.LittleEndian.PutUint32(buf[40:], uint32(dataSize))
	// 静音：PCM 全 0（已在 buf 里）
	return buf
}

var wav = wavSilence()

// searchItems 固定搜索结果（>=5 条，供 e2e-smoke 步骤2 断言）
func searchItems() []map[string]any {
	base := []map[string]any{
		{"bvid": "BV1xx411c7mD", "title": "E2E 测试歌曲一", "author": "测试歌手", "pic": "http://x/1.jpg", "duration": "04:12", "play": 12345, "typename": "视频"},
		{"bvid": "BV1xx411c7mE", "title": "E2E 测试歌曲二", "author": "测试歌手", "pic": "http://x/2.jpg", "duration": "03:45", "play": 6789, "typename": "视频"},
		{"bvid": "BV1xx411c7mF", "title": "E2E 测试歌曲三", "author": "测试歌手", "pic": "http://x/3.jpg", "duration": "05:01", "play": 111, "typename": "视频"},
		{"bvid": "BV1xx411c7mG", "title": "E2E 测试歌曲四", "author": "测试歌手", "pic": "http://x/4.jpg", "duration": "02:30", "play": 222, "typename": "视频"},
		{"bvid": "BV1xx411c7mH", "title": "E2E 测试歌曲五", "author": "测试歌手", "pic": "http://x/5.jpg", "duration": "06:00", "play": 333, "typename": "视频"},
	}
	return base
}

func main() {
	mux := http.NewServeMux()

	// 搜索
	mux.HandleFunc("/x/web-interface/search/type", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"code": 0, "message": "success", "data": map[string]any{
			"numResults": len(searchItems()), "result": searchItems(),
		}})
	})

	// 视频信息（拿 cid）
	mux.HandleFunc("/x/web-interface/view", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"code": 0, "message": "success", "data": map[string]any{
			"bvid": r.URL.Query().Get("bvid"), "aid": 1, "cid": 123, "title": "E2E 测试",
			"duration": 250, "pages": []map[string]any{{"cid": "123", "part": "E2E 测试"}},
		}})
	})

	// playurl：DASH 音频 baseUrl 指回本 server 的 /audio.m4s
	mux.HandleFunc("/x/player/playurl", func(w http.ResponseWriter, r *http.Request) {
		host := r.Host // 本 server 地址
		writeJSON(w, map[string]any{"code": 0, "message": "success", "data": map[string]any{
			"quality": 30216, "format": "dash",
			"dash": map[string]any{"audio": []map[string]any{{
				"id": 30216, "baseUrl": "http://" + host + "/audio.m4s",
				"bandwidth": 64000, "mimeType": "audio/wav", "codecs": "pcm", "size": len(wav),
			}}},
		}})
	})

	// 音频流（真实可播静音 WAV）
	mux.HandleFunc("/audio.m4s", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		w.Header().Set("Content-Length", strconv.Itoa(len(wav)))
		w.Header().Set("Access-Control-Allow-Origin", "*")
		// Range 支持（前端 audio 可能发 Range）
		if rng := r.Header.Get("Range"); rng != "" {
			w.WriteHeader(http.StatusPartialContent)
			w.Header().Set("Content-Range", "bytes 0-" + strconv.Itoa(len(wav)-1) + "/" + strconv.Itoa(len(wav)))
		}
		w.Write(wav)
	})

	log.Printf("mockbili listening on http://127.0.0.1%s", port)
	log.Fatal(http.ListenAndServe(port, mux))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Write([]byte(mustJSON(v)))
}

func mustJSON(v any) string {
	// 简单 JSON 序列化（无第三方依赖，用 encoding/json）
	b, err := json.Marshal(v)
	if err != nil {
		return `{"code":500,"message":"json err"}`
	}
	return string(b)
}
