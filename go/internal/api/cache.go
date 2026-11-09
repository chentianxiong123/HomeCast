// Package api 缓存端点：/api/v1/cache/* 供前端设置页/播放器调用。
package api

import (
	"encoding/json"
	"net/http"

	"homecast/internal/service/cache"
)

// CacheHandler 音频缓存端点
type CacheHandler struct {
	Cache    *cache.CacheService
	FavTable string // 收藏表名（ClearUnfav 用）
}

// Stats GET /api/v1/cache/stats —— 已用大小/条数/上限
func (h *CacheHandler) Stats(w http.ResponseWriter, r *http.Request) {
	ok(w, map[string]any{
		"used_mb":  h.Cache.Size(),
		"count":    h.Cache.Count(),
		"limit_mb": h.Cache.LimitMB(),
	})
}

// ClearAll POST /api/v1/cache/clear?mode=all|unfav —— 全清 / 只清未收藏
func (h *CacheHandler) ClearAll(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	if mode == "unfav" {
		h.Cache.ClearUnfav(h.FavTable)
	} else {
		h.Cache.ClearAll()
	}
	ok(w, true)
}

// SetLimit POST /api/v1/cache/limit body {limit_mb} —— 更新上限（0=关）
func (h *CacheHandler) SetLimit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		LimitMB int `json:"limit_mb"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errResp(w, http.StatusBadRequest, "invalid body")
		return
	}
	h.Cache.SetLimitMB(body.LimitMB)
	ok(w, true)
}

// Limit GET /api/v1/cache/limit —— 当前上限
func (h *CacheHandler) Limit(w http.ResponseWriter, r *http.Request) {
	ok(w, map[string]any{"limit_mb": h.Cache.LimitMB()})
}
