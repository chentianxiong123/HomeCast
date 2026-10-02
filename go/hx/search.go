// 搜索功能切片：handler + 渲染 + 分页，全部在这一个文件
package hx

import (
	"bytes"
	"html/template"
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

// searchPageData 搜索完整页数据：结果 HTML 预渲染注入 #results（直接访问 /hx/search?kw= 用）
type searchPageData struct {
	Keyword     string
	ResultsHTML template.HTML // 空 = 空态
}

// searchFragment 渲染结果片段 → HTML（Search 与完整页共用，面向过程组合）
func (h *H) searchFragment(kw string, page int) (template.HTML, error) {
	res, err := h.Music.Search(kw, page, 20)
	if err != nil {
		return "", err
	}
	v := &searchView{
		Keyword:  kw,
		Total:    res.Total,
		Page:     page,
		NextPage: page + 1,
		HasMore:  len(res.List) >= 20,
		Items:    res.List,
	}
	var buf bytes.Buffer
	if err := h.Tpl.ExecuteTemplate(&buf, "results.html", v); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

// Search GET /hx/search?kw=&page=
//   - htmx 请求（带 HX-Request 头）→ 结果片段（替换 #results）
//   - 直接访问 → 完整页（shell + 搜索框 + 预渲染结果），刷新/直达可用
func (h *H) Search(w http.ResponseWriter, r *http.Request) {
	kw := strings.TrimSpace(r.URL.Query().Get("kw"))
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	isHX := r.Header.Get("HX-Request") != ""

	if kw == "" {
		if isHX {
			h.Tpl.ExecuteTemplate(w, "results.html", &searchView{})
			return
		}
		h.searchPage(w, r)
		return
	}

	frag, err := h.searchFragment(kw, page)
	if err != nil {
		http.Error(w, "搜索失败："+err.Error(), http.StatusInternalServerError)
		return
	}
	if isHX {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(frag))
		return
	}
	h.renderPage(w, "search", "content_search.html", &searchPageData{Keyword: kw, ResultsHTML: frag})
}