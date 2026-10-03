package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"homecast/internal/service"
	"homecast/internal/store"
)

// newTestFavHandler 测试用 fav 栈（临时 db）
func newTestFavHandler(t *testing.T) (*FavHandler, http.Handler) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(db, t.TempDir()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	fav := &FavHandler{Fav: service.NewFavService(db)}
	mux := http.NewServeMux()
	// 独立组装 fav 端点（对齐 NewMux 中的注册方式）
	mux.HandleFunc("GET /api/v1/fav/list", fav.List)
	mux.HandleFunc("POST /api/v1/fav/add", fav.Add)
	mux.HandleFunc("DELETE /api/v1/fav/{bvid}", fav.Remove)
	mux.HandleFunc("POST /api/v1/fav/clear", fav.Clear)
	return fav, mux
}

func doReq(t *testing.T, h http.Handler, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != nil {
		r = httptest.NewRequest(method, path, bytes.NewReader(body))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// TestFavEndpoints 全端点行为：add → list → delete → clear + 参数校验
func TestFavEndpoints(t *testing.T) {
	_, mux := newTestFavHandler(t)

	// 空 body / 缺 bvid → 400
	if w := doReq(t, mux, "POST", "/api/v1/fav/add", nil); w.Code != http.StatusBadRequest {
		t.Errorf("add 空body code=%d, want 400", w.Code)
	}
	if w := doReq(t, mux, "POST", "/api/v1/fav/add", []byte(`{"title":"x"}`)); w.Code != http.StatusBadRequest {
		t.Errorf("add 缺bvid code=%d, want 400", w.Code)
	}

	// add 正常
	payload, _ := json.Marshal(service.FavSong{BVID: "BV1", Title: "一", Artist: "a", Cover: "c", Duration: "3:00", DurationSec: 180})
	if w := doReq(t, mux, "POST", "/api/v1/fav/add", payload); w.Code != http.StatusOK {
		t.Errorf("add code=%d, want 200", w.Code)
	}

	// list 包含 BV1（响应是 {code,data,message} 信封）
	var resp struct {
		Code int                `json:"code"`
		Data []service.FavSong  `json:"data"`
	}
	w := doReq(t, mux, "GET", "/api/v1/fav/list", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list code=%d, want 200", w.Code)
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != 0 || len(resp.Data) != 1 || resp.Data[0].BVID != "BV1" {
		t.Errorf("list = %+v, want [BV1]", resp.Data)
	}

	// delete 存在
	if w := doReq(t, mux, "DELETE", "/api/v1/fav/BV1", nil); w.Code != http.StatusOK {
		t.Errorf("delete code=%d, want 200", w.Code)
	}
	// delete 不存在（幂等）
	if w := doReq(t, mux, "DELETE", "/api/v1/fav/BV999", nil); w.Code != http.StatusOK {
		t.Errorf("delete 不存在 code=%d, want 200", w.Code)
	}

	// clear
	if w := doReq(t, mux, "POST", "/api/v1/fav/clear", nil); w.Code != http.StatusOK {
		t.Errorf("clear code=%d, want 200", w.Code)
	}
	w = doReq(t, mux, "GET", "/api/v1/fav/list", nil)
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Data) != 0 {
		t.Errorf("clear 后 list len=%d, want 0", len(resp.Data))
	}

	// 方法不对 → 405（GET-only 端点用 POST 打）
	if w := doReq(t, mux, "POST", "/api/v1/fav/list", nil); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST fav/list code=%d, want 405", w.Code)
	}
}

// TestLyricParams 歌词参数校验（无外网路径）
func TestLyricParams(t *testing.T) {
	h := &LyricHandler{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/music/lyric", h.Get)
	mux.HandleFunc("GET /api/v1/music/lyric/candidates", h.Candidates)

	// 无参 → 400
	if w := doReq(t, mux, "GET", "/api/v1/music/lyric", nil); w.Code != http.StatusBadRequest {
		t.Errorf("lyric 无参 code=%d, want 400", w.Code)
	}
	// candidates 无 keyword → 400
	if w := doReq(t, mux, "GET", "/api/v1/music/lyric/candidates", nil); w.Code != http.StatusBadRequest {
		t.Errorf("candidates 无参 code=%d, want 400", w.Code)
	}
}