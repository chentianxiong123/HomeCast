// 搜索功能切片：handler + 渲染 + 分页，全部在这一个文件
package hx

import (
	"net/http"
	"strconv"
	"strings"

	"homecast/internal/service"
)

// searchView 搜索结果视图数据（页面看到什么，这里就是什么）
type searchView struct {
	Keyword  string
	Total    int
	Page     int
	NextPage int
	HasMore  bool
	Items    []service.MusicItem
}

// Search GET /hx/search?kw=&page= → 渲染 results.html 片段，htmx 替换 #results
func (h *H) Search(w http.ResponseWriter, r *http.Request) {
	kw := strings.TrimSpace(r.URL.Query().Get("kw"))
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	if kw == "" {
		h.Tpl.ExecuteTemplate(w, "results.html", &searchView{})
		return
	}

	res, err := h.Music.Search(kw, page, 20)
	if err != nil {
		http.Error(w, "搜索失败："+err.Error(), http.StatusInternalServerError)
		return
	}

	v := &searchView{
		Keyword:  kw,
		Total:    res.Total,
		Page:     page,
		NextPage: page + 1,
		HasMore:  len(res.List) >= 20,
		Items:    res.List,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.Tpl.ExecuteTemplate(w, "results.html", v)
}