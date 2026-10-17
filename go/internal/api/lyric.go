package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"homecast/internal/service/lyric"
	"homecast/internal/service/music"
)

// LyricHandler 歌词端点（网易云源）
type LyricHandler struct {
	Lyric *lyric.LyricStore
	Music *music.MusicService // bvid → 分P标题作搜索关键词（B 站合集痛点；带 view 缓存）
}

// keywordsFor bvid 非空时优先分P标题列表（bili-music lyricSearchTitles 同款，逐个搜第一个命中），
// 否则原关键词清洗
func (h *LyricHandler) keywordsFor(bvid, keyword string) []string {
	if h.Music != nil && bvid != "" {
		return h.Music.LyricKeywords(bvid, keyword)
	}
	if k := lyric.CleanSearchKeyword(keyword); k != "" {
		return []string{k}
	}
	return nil
}

// Get GET /api/v1/music/lyric?keyword=&sid=&bvid=
// bvid 优先：有已选歌词源（前端切源上报持久化）就用已选源——桌面歌词挂件（只带 bvid/keyword）
// 与歌词页（切源后）拿到同一份歌词，两端同步；bvid 模式同时返回本歌存档的对齐偏移。
func (h *LyricHandler) Get(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sid, _ := strconv.Atoi(q.Get("sid"))
	bvid := strings.TrimSpace(q.Get("bvid"))
	keywords := h.keywordsFor(bvid, q.Get("keyword"))
	if len(keywords) == 0 && sid == 0 {
		errResp(w, http.StatusBadRequest, "keyword or sid required")
		return
	}
	if bvid != "" {
		if sel := h.Lyric.SelectedSID(bvid); sel > 0 {
			sid = sel
		}
	}
	res := lyric.GetLyricLines(keywords, sid)
	if res == nil {
		// 找不到歌词是正常业务，code=0 data=null（前端拦截器按 code!=0 弹错）
		ok(w, nil)
		return
	}
	if bvid != "" {
		if sel, ok := h.Lyric.Get(bvid); ok {
			res.Offset = sel.Offset
		}
	}
	ok(w, res)
}

// Meta GET /api/v1/lyric/meta?bvid= → 已存选择 {sid,name,artist,duration,offset}，无则 null
// 轻量端点：歌词页/字幕条打开时取“选的是哪个候选 + 对齐偏移”，不拉歌词正文。
func (h *LyricHandler) Meta(w http.ResponseWriter, r *http.Request) {
	bvid := strings.TrimSpace(r.URL.Query().Get("bvid"))
	if bvid == "" {
		errResp(w, http.StatusBadRequest, "bvid required")
		return
	}
	sel, found := h.Lyric.Get(bvid)
	if !found {
		ok(w, nil) // 完全无记录（既没选候选也没调过偏移）才 null
		return
	}
	ok(w, map[string]any{
		"sid":      sel.SID,
		"name":     sel.Name,
		"artist":   sel.Artist,
		"duration": sel.Duration,
		"offset":   sel.Offset,
	})
}

// Select POST /api/v1/lyric/select  body 或 query {bvid, sid[, name, artist, duration]}
// 歌词页切源上报（持久化 SQLite），挂件随动；sid<=0 清除记录。
// 安卓壳（wails3 asset 桥）丢弃 POST body，参数走 query；桌面/浏览器 body 也可
func (h *LyricHandler) Select(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BVID     string `json:"bvid"`
		SID      int    `json:"sid"`
		Name     string `json:"name"`
		Artist   string `json:"artist"`
		Cover    string `json:"cover"`
		Duration int    `json:"duration"`
	}
	body.BVID = strings.TrimSpace(r.FormValue("bvid"))
	body.SID, _ = strconv.Atoi(r.FormValue("sid"))
	body.Name = r.FormValue("name")
	body.Artist = r.FormValue("artist")
	body.Cover = r.FormValue("cover")
	body.Duration, _ = strconv.Atoi(r.FormValue("duration"))
	if body.BVID == "" {
		// 兼容 JSON body（桌面/网页）
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body.BVID != "" {
			// 已从 query 读到则 body 不覆盖（query 优先，防安卓壳丢 body 时双重空）
		}
	}
	if body.BVID == "" {
		errResp(w, http.StatusBadRequest, "bvid required")
		return
	}
	h.Lyric.SetSelection(body.BVID, lyric.LyricSel{
		BVID: body.BVID, SID: body.SID, Name: body.Name,
		Artist: body.Artist, Cover: body.Cover, Duration: body.Duration,
	})
	ok(w, true)
}

// Offset POST /api/v1/lyric/offset  body 或 query {bvid, offset}
// 歌词 ±0.5s 手动对齐：偏移持久化（music.py _nudge_lyric 同款语义，存 SQLite）
func (h *LyricHandler) Offset(w http.ResponseWriter, r *http.Request) {
	bvid := strings.TrimSpace(r.FormValue("bvid"))
	offset, _ := strconv.ParseFloat(r.FormValue("offset"), 64)
	if bvid == "" {
		var body struct {
			BVID   string  `json:"bvid"`
			Offset float64 `json:"offset"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil && body.BVID != "" {
			bvid, offset = body.BVID, body.Offset
		}
	}
	if bvid == "" {
		errResp(w, http.StatusBadRequest, "bvid required")
		return
	}
	h.Lyric.SetOffset(bvid, offset)
	ok(w, true)
}

// Candidates GET /api/v1/music/lyric/candidates?keyword=&limit=
func (h *LyricHandler) Candidates(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	keywords := h.keywordsFor(q.Get("bvid"), q.Get("keyword"))
	if len(keywords) == 0 {
		errResp(w, http.StatusBadRequest, "keyword required")
		return
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit == 0 {
		limit = 8
	}
	ok(w, lyric.SearchLyricCandidatesByTitles(keywords, limit))
}