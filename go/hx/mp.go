// 小程序搜索片段：给 mp-html 渲染的纯展示 HTML（无 onclick/hx-post）
// mp-html 不执行 JS，但支持 <a> 的 link 事件 —— 卡片用 <a href="hc-play://..."> 包裹，
// 小程序逻辑层在 mp-html 的 @link 事件里拿 href 解析 bvid 触发播放。
package hx

import (
	"bytes"
	"html/template"
	"net/http"
	"strings"

	"homecast/internal/service/music"
)

// searchViewMP 小程序搜索结果视图（复用 searchView 数据）
type searchViewMP struct {
	Keyword string
	Total   int
	Items   []music.MusicItem
}

// SearchMP GET /hx/search-mp?kw=
//   - 小程序专用：返回纯展示片段，无 onclick / hx-* / 内联 <script>
//   - 卡片 = <a href="hc-play://play?bvid=..."> 包裹（mp-html 渲原生 + link 事件）
func (h *H) SearchMP(w http.ResponseWriter, r *http.Request) {
	kw := strings.TrimSpace(r.URL.Query().Get("kw"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if kw == "" {
		h.Tpl.ExecuteTemplate(w, "results_mp.html", &searchViewMP{})
		return
	}
	frag, err := h.searchFragmentMP(kw)
	if err != nil {
		http.Error(w, "搜索失败："+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write([]byte(frag))
}

// searchFragmentMP 渲染小程序搜索结果片段（一次 50 条，整体排序，同 web 版不做分页）
func (h *H) searchFragmentMP(kw string) (template.HTML, error) {
	res, err := h.Music.Search(kw, 1, 50)
	if err != nil {
		return "", err
	}
	v := &searchViewMP{Keyword: kw, Total: len(res.List), Items: res.List}
	var buf bytes.Buffer
	if err := h.Tpl.ExecuteTemplate(&buf, "results_mp.html", v); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}
