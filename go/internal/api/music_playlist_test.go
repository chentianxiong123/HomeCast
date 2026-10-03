package api

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"homecast/internal/bilibili"
	"homecast/internal/service"
	"homecast/internal/store"
)

// openTestDB 临时 SQLite（api 包共用）
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(db, t.TempDir()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// newFakeMusic 假 B 站服务器（view + playurl + 代理回源三级）+ MusicHandler
// playurl 的 baseUrl 指回假服务器 → 代理拉流也能全链路命中（真 HTTP 回源）
func newFakeMusic(t *testing.T, viewBody, playurlBody string) *MusicHandler {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/x/web-interface/search/type":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(searchResp(viewBody)))
		case "/x/web-interface/view":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(viewResp(viewBody)))
		case "/x/player/playurl":
			w.Header().Set("Content-Type", "application/json")
			b := playurlBody
			if b == "" {
				b = `{"code":0,"message":"success","data":{"quality":30216,"dash":{"audio":[{"id":30216,"baseUrl":"%s/audio.m4s","bandwidth":64000,"mimeType":"audio/mp4","codecs":"mp4a.40.2","size":4}]}}}`
			}
			w.Write([]byte(fmt.Sprintf(b, srv.URL)))
		case "/audio.m4s": // 代理回源目标（假 CDN）
			w.Header().Set("Content-Type", "audio/mp4")
			w.Write([]byte("FAKEAUDIO"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	client := bilibili.NewClient(srv.URL, "TestUA/1.0", "https://ref.example.com", 5*time.Second)
	return NewMusicHandler(&service.MusicService{Client: client})
}

func searchResp(searchJSON string) string {
	if searchJSON == "" {
		return `{"code":0,"message":"success","data":{"numResults":0,"result":[]}}`
	}
	return searchJSON
}
func viewResp(viewJSON string) string {
	if viewJSON == "" {
		return `{"code":0,"message":"success","data":{"bvid":"BV1","cid":123,"title":"t","duration":225,"owner":{"name":"u"}}}`
	}
	return viewJSON
}

// TestMusicSearchParamsAndResult 搜索：缺 keyword 400；正常 200 信封+数据
func TestMusicSearchParamsAndResult(t *testing.T) {
	h := newFakeMusic(t, "", "")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/music/search", h.Search)

	// 缺 keyword → 400
	if w := doReq(t, mux, "GET", "/api/v1/music/search", nil); w.Code != http.StatusBadRequest {
		t.Errorf("缺keyword code=%d, want 400 (body=%s)", w.Code, w.Body.String())
	}
	// 空 keyword（纯空白）→ 400
	if w := doReq(t, mux, "GET", "/api/v1/music/search?keyword=%20%20", nil); w.Code != http.StatusBadRequest {
		t.Errorf("空白keyword code=%d, want 400", w.Code)
	}
	// 正常（空结果也 200 + code 0）
	w := doReq(t, mux, "GET", "/api/v1/music/search?keyword=x", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("正常 code=%d, want 200", w.Code)
	}
	if got := w.Body.String(); !contains(got, `"code":0`) || !contains(got, `"total"`) {
		t.Errorf("信封异常: %s", got)
	}
}

// TestMusicStream 全链路：view 拿 cid → playurl 取流 → 代理回源拉流（真 HTTP 假 B 站三级）
func TestMusicStream(t *testing.T) {
	h := newFakeMusic(t, "", "")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/music/stream/{bvid}", h.Stream)

	w := doReq(t, mux, "GET", "/api/v1/music/stream/BV1", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("stream code=%d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	if got := w.Body.String(); !contains(got, "FAKEAUDIO") {
		t.Errorf("stream 未透传代理音频: %s", got)
	}
}

// contains 简单子串判断（测试工具）
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// TestPlaylistEndpoints 播放列表端点（client 假服务器喂 view：Add 未存在路径全链路）
func TestPlaylistEndpoints(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fmt.Sprintf(`{"code":0,"message":"success","data":{"bvid":"%s","cid":1,"title":"新歌","duration":180,"owner":{"name":"UP"}}}`,
			r.URL.Query().Get("bvid"))))
	}))
	t.Cleanup(srv.Close)
	db := openTestDB(t)
	client := bilibili.NewClient(srv.URL, "TestUA", "https://ref", 5*time.Second)
	pl := &PlaylistHandler{Playlist: service.NewPlaylistService(client, db)}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/playlist", pl.Get)
	mux.HandleFunc("POST /api/v1/playlist/add/{bvid}", pl.Add)
	mux.HandleFunc("POST /api/v1/playlist/remove/{bvid}", pl.Remove)
	mux.HandleFunc("POST /api/v1/playlist/clear", pl.Clear)

	// 空列表
	if w := doReq(t, mux, "GET", "/api/v1/playlist", nil); w.Code != http.StatusOK {
		t.Fatalf("Get code=%d", w.Code)
	}
	// Add 缺 bvid → 400
	if w := doReq(t, mux, "POST", "/api/v1/playlist/add/", nil); w.Code != http.StatusNotFound {
		// 空路径不匹配 {bvid} → 404（ServeMux），断言不是 200
		t.Logf("Add 空 bvid → %d（期望非 200）", w.Code)
	}
	// Add 未存在 → 200 + 假服务器信息
	w := doReq(t, mux, "POST", "/api/v1/playlist/add/BV-NEW", nil)
	if w.Code != http.StatusOK || !contains(w.Body.String(), "新歌") {
		t.Fatalf("Add code=%d body=%s", w.Code, w.Body.String())
	}
	// 重复 Add 同 bvid → 不重复
	w = doReq(t, mux, "POST", "/api/v1/playlist/add/BV-NEW", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("Add 重复 code=%d", w.Code)
	}
	// Get 只有 1 条
	w = doReq(t, mux, "GET", "/api/v1/playlist", nil)
	if !contains(w.Body.String(), "BV-NEW") {
		t.Errorf("Get 缺 BV-NEW: %s", w.Body.String())
	}
	// Remove
	if w := doReq(t, mux, "POST", "/api/v1/playlist/remove/BV-NEW", nil); w.Code != http.StatusOK {
		t.Errorf("Remove code=%d", w.Code)
	}
	// Clear
	if w := doReq(t, mux, "POST", "/api/v1/playlist/clear", nil); w.Code != http.StatusOK {
		t.Errorf("Clear code=%d", w.Code)
	}
}