package api

import (
	"net/http"
	"strconv"
	"strings"

	"homecast/internal/service"
)

// LyricHandler 歌词端点（网易云源）
type LyricHandler struct{}

// Get GET /api/v1/music/lyric?keyword=&sid=
func (h *LyricHandler) Get(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	keyword := strings.TrimSpace(q.Get("keyword"))
	sid, _ := strconv.Atoi(q.Get("sid"))
	if keyword == "" && sid == 0 {
		errResp(w, http.StatusBadRequest, "keyword or sid required")
		return
	}
	res := service.GetLyricLines(keyword, sid)
	if res == nil {
		// 找不到歌词是正常业务，code=0 data=null（前端拦截器按 code!=0 弹错）
		ok(w, nil)
		return
	}
	ok(w, res)
}

// Candidates GET /api/v1/music/lyric/candidates?keyword=&limit=
func (h *LyricHandler) Candidates(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	keyword := strings.TrimSpace(q.Get("keyword"))
	if keyword == "" {
		errResp(w, http.StatusBadRequest, "keyword required")
		return
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit == 0 {
		limit = 8
	}
	ok(w, service.SearchLyricCandidates(keyword, limit))
}