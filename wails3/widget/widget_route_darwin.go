//go:build darwin

// 桌面挂件显示/隐藏路由（Linux GTK4）：下面面板 #d-tick → POST /api/v1/widget/lyric-visible {on}
// 平台分离（解耦）：只有 linux 才 import widget 包（cgo GTK）；安卓/其他平台 no-op。
package main

import (
	"encoding/json"
	"net/http"

	"homecast/app/widget"
)

func wrapWidgetRoute(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/widget/lyric-visible" && r.Method == "POST" {
			var body struct {
				On bool `json:"on"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			widget.SetVisible(body.On)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"data":true,"message":"success"}`))
			return
		}
		h.ServeHTTP(w, r)
	})
}
