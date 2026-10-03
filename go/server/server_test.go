package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestWithCORS 跨域包装：OPTIONS 预检 204 + CORS 头（桌面壳 wails:// 域依赖）
func TestWithCORS(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("hi"))
	})
	h := withCORS(inner)

	// OPTIONS 预检 → 204，不落到 handler
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/fav/list", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Errorf("OPTIONS code=%d, want 204", w.Code)
	}

	// 正常请求带 CORS 头
	req = httptest.NewRequest(http.MethodGet, "/api/v1/fav/list", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Allow-Origin=%q, want *", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("Allow-Methods 头缺失")
	}
	if w.Body.String() != "hi" {
		t.Errorf("body=%q, want hi（应透传给内层）", w.Body.String())
	}
}