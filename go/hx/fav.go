// 收藏功能切片（M2）：收藏夹页 + 收藏/取消（服务端决定存在性 → 可重绘）
// 数据走现有 JSON 服务（去重置顶，新的在前）
package hx

import (
	"net/http"
	"strconv"

	"homecast/internal/service"
)

// favPageData 收藏夹页数据
type favPageData struct {
	Items []service.FavSong
}

// favBtnView 收藏按钮片段数据（toggle 返回 hx-vals 保持可再点）
type favBtnView struct {
	BVID     string
	Title    string
	Artist   string
	Cover    string
	Duration int
	Faved    bool
}

// FavsPage GET /hx/favs → 收藏夹页
func (h *H) FavsPage(w http.ResponseWriter, r *http.Request) {
	list := h.Fav.List()
	h.renderPage(w, "favs", "content_favs.html", &favPageData{Items: list})
}

// FavToggle POST /hx/fav/toggle → 在则取消、不在则收藏；返回按钮片段（outerHTML 替换）
func (h *H) FavToggle(w http.ResponseWriter, r *http.Request) {
	v := &favBtnView{
		BVID:   r.FormValue("bvid"),
		Title:  r.FormValue("title"),
		Artist: r.FormValue("artist"),
		Cover:  r.FormValue("cover"),
	}
	v.Duration, _ = strconv.Atoi(r.FormValue("duration"))
	if v.BVID == "" {
		http.Error(w, "bvid required", http.StatusBadRequest)
		return
	}

	faved := false
	for _, f := range h.Fav.List() {
		if f.BVID == v.BVID {
			faved = true
			break
		}
	}
	if faved {
		h.Fav.Remove(v.BVID)
	} else {
		h.Fav.Add(service.FavSong{
			BVID:        v.BVID,
			Title:       v.Title,
			Artist:      v.Artist,
			Cover:       v.Cover,
			Duration:    secondsToStr(v.Duration),
			DurationSec: v.Duration,
		})
	}
	v.Faved = !faved

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.Tpl.ExecuteTemplate(w, "fav_btn.html", v)
}

// FavRemove POST /hx/fav/remove/{bvid} → 返回收藏列表片段替换 #fav-list
func (h *H) FavRemove(w http.ResponseWriter, r *http.Request) {
	bvid := r.PathValue("bvid")
	if bvid == "" {
		http.Error(w, "bvid required", http.StatusBadRequest)
		return
	}
	h.Fav.Remove(bvid)
	h.renderFavList(w)
}

// FavClear POST /hx/fav/clear → 清空并返回收藏列表片段
func (h *H) FavClear(w http.ResponseWriter, r *http.Request) {
	h.Fav.Clear()
	h.renderFavList(w)
}

// renderFavList 渲染收藏列表片段（共用零件）
func (h *H) renderFavList(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.Tpl.ExecuteTemplate(w, "fav_list.html", &favPageData{Items: h.Fav.List()})
}

// secondsToStr 秒 → "m:ss"（与 service.FormatDuration 一致，避免导入环）
func secondsToStr(sec int) string {
	if sec <= 0 {
		return ""
	}
	return strconv.Itoa(sec/60) + ":" + pad2(sec%60)
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}