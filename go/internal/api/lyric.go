package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"homecast/internal/service"
)

// LyricHandler 歌词端点（网易云源）
type LyricHandler struct{}

// Get GET /api/v1/music/lyric?keyword=&sid=&bvid=
// bvid 优先：有已选歌词源（前端切源上报）就用已选源——桌面歌词挂件（只带 bvid/keyword）
// 与歌词页（切源后）拿到同一份歌词，两端同步。
func (h *LyricHandler) Get(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	keyword := strings.TrimSpace(q.Get("keyword"))
	sid, _ := strconv.Atoi(q.Get("sid"))
	bvid := strings.TrimSpace(q.Get("bvid"))
	if keyword == "" && sid == 0 && bvid == "" {
		errResp(w, http.StatusBadRequest, "keyword or sid required")
		return
	}
	if bvid != "" {
		if sel := service.SelectedSID(bvid); sel > 0 {
			sid = sel
		}
	}
	res := service.GetLyricLines(keyword, sid)
	if res == nil {
		// 找不到歌词是正常业务，code=0 data=null（前端拦截器按 code!=0 弹错）
		ok(w, nil)
		return
	}
	ok(w, res)
}

// Select POST /api/v1/lyric/select?bvid=&sid= 或 body {bvid, sid}——歌词页切源上报，挂件随动
// 安卓壳（wails3 asset 桥）丢弃 POST body，参数走 query；桌面/浏览器 body 也可
func (h *LyricHandler) Select(w http.ResponseWriter, r *http.Request) {
	bvid := strings.TrimSpace(r.FormValue("bvid"))
	sid, _ := strconv.Atoi(r.FormValue("sid"))
	if bvid == "" {
		// 兼容 JSON body（桌面/网页）
		var body struct {
			BVID string `json:"bvid"`
			SID  int    `json:"sid"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body.BVID != "" {
			bvid, sid = body.BVID, body.SID
		}
	}
	if bvid == "" {
		errResp(w, http.StatusBadRequest, "bvid required")
		return
	}
	service.SetLyricSelection(bvid, sid)
	ok(w, true)
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