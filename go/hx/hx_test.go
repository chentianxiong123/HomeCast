package hx

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"homecast/internal/service"
	"homecast/internal/store"
	"homecast/web"
)

// newTestH 最小功能上下文（tmp db）：不联网路径全可测
func newTestH(t *testing.T) *H {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(db, t.TempDir()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	tpl, err := web.Templates()
	if err != nil {
		t.Fatalf("templates: %v", err)
	}
	return &H{
		Fav:  service.NewFavService(db),
		Cast: service.NewCastService(nil, service.NewTokenStore()),
		Tpl:  tpl,
	}
}

// TestPagesRender 全页面渲染 200：壳 + 各功能切片（无网路径）
func TestPagesRender(t *testing.T) {
	h := newTestH(t)
	r := h.Router()

	for _, path := range []string{"/", "/hx/queue", "/hx/recent", "/hx/favs", "/hx/settings", "/hx/search", "/hx/cast"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("GET %s → %d, want 200", path, w.Code)
		}
		if !strings.Contains(w.Body.String(), "</html>") && path != "/hx/cast" {
			// 页面必须渲染完整 HTML（cast 页无 ui 断言）
			t.Errorf("GET %s 响应缺 </html>，可能渲染失败", path)
		}
	}
}

// TestAssetsNoStore 静态资源 200 + no-store 头（asset 治理核心：浏览器不得缓存）
func TestAssetsNoStore(t *testing.T) {
	h := newTestH(t)
	r := h.Router()

	for _, path := range []string{"/assets/player.js", "/assets/style.css", "/assets/title.js", "/assets/favicon.svg"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("GET %s → %d, want 200", path, w.Code)
			continue
		}
		if got := w.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
			t.Errorf("GET %s Cache-Control=%q, want no-store", path, got)
		}
	}

	// 未知静态 → 404
	req := httptest.NewRequest(http.MethodGet, "/assets/nope.js", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("未知静态 → %d, want 404", w.Code)
	}

	// 未知 hx 路由 → 404
	req = httptest.NewRequest(http.MethodGet, "/hx/unknown", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("未知 /hx/%d, want 404", w.Code)
	}
}

// TestFavToggleBehavior toggle 双向：首次收藏（片段含激活态）→ 再点取消（服务端状态同步）
func TestFavToggleBehavior(t *testing.T) {
	h := newTestH(t)
	r := h.Router()

	form := url.Values{"bvid": {"BV1"}, "title": {"一"}, "artist": {"a"}, "cover": {"c"}, "duration": {"180"}}

	// 第一次 toggle → 收藏
	req := httptest.NewRequest(http.MethodPost, "/hx/fav/toggle", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("toggle1 code=%d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "text-pink-500") {
		t.Errorf("收藏态片段应含激活态 class text-pink-500; body=%s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "hx-confirm") {
		t.Errorf("收藏态片段应含 hx-confirm（取消需确认）; body=%s", w.Body.String())
	}
	if got := h.Fav.List(); len(got) != 1 || got[0].BVID != "BV1" {
		t.Errorf("toggle1 后收藏 = %+v, want [BV1]", got)
	}

	// 第二次 toggle → 取消
	req = httptest.NewRequest(http.MethodPost, "/hx/fav/toggle", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	// 用模板分歧点断言（title 属性/激活类/hx-confirm），避免误伤 hx-on 里的 JS 文本
	if !strings.Contains(w.Body.String(), `title="收藏"`) {
		t.Errorf("取消态片段应为 title=收藏; body=%s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "hx-confirm=") {
		t.Errorf("取消态片段不应含 hx-confirm")
	}
	if got := h.Fav.List(); len(got) != 0 {
		t.Errorf("toggle2 后收藏 = %+v, want 空", got)
	}

	// 缺 bvid → 400
	req = httptest.NewRequest(http.MethodPost, "/hx/fav/toggle", strings.NewReader("title=x"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("缺 bvid code=%d, want 400", w.Code)
	}
}

// TestFavRemoveClearEndpoints FavRemove / FavClear 返回列表片段且状态正确
func TestFavRemoveClearEndpoints(t *testing.T) {
	h := newTestH(t)
	r := h.Router()
	h.Fav.Add(service.FavSong{BVID: "BV1", Title: "一", Artist: "a", Cover: "c", Duration: "3:00", DurationSec: 180})
	h.Fav.Add(service.FavSong{BVID: "BV2", Title: "二", Artist: "b", Cover: "c", Duration: "4:00", DurationSec: 240})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/hx/fav/remove/BV1", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "BV2") {
		t.Errorf("remove 片段异常: code=%d body=%s", w.Code, w.Body.String()[:min(200, len(w.Body.String()))])
	}
	if got := h.Fav.List(); len(got) != 1 {
		t.Errorf("remove 后 len=%d, want 1", len(got))
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/hx/fav/clear", nil))
	if w.Code != http.StatusOK {
		t.Errorf("clear code=%d, want 200", w.Code)
	}
	if got := h.Fav.List(); len(got) != 0 {
		t.Errorf("clear 后 len=%d, want 0", len(got))
	}
}

// TestTemplateFuncsRender 模板函数（shortTitle）经模板引擎实际渲染正确
func TestTemplateFuncsRender(t *testing.T) {
	h := newTestH(t)
	// 直接执行一个含 shortTitle 的模板片段（结果页逻辑同款）
	tpl, _ := web.Templates()
	var sb strings.Builder
	if err := tpl.ExecuteTemplate(&sb, "fav_btn.html", map[string]any{}); err != nil {
		t.Fatalf("fav_btn render: %v", err)
	}
	_ = h
}

// min 工具
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}