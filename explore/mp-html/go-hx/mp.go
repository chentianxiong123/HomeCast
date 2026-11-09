// 小程序搜索片段：给 mp-html 渲染的纯展示 HTML（无 onclick/hx-post）
// mp-html 不执行 JS，但支持 <a> 的 link 事件 —— 卡片用 <a href="hc-play://..."> 包裹，
// 小程序逻辑层在 mp-html 的 @link 事件里拿 href 解析 bvid 触发播放。
package hx

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

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

// LyricPanel GET /hx/lyric-panel —— 实验：动态歌词面板（测 mp-html 高频刷新 + 多节点滚动流畅度）
// 模拟 12 行歌词，当前行随时间滚动高亮（红色加粗），验证 mp-html 每秒刷 12+ 节点是否卡
func (h *H) LyricPanel(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	now := time.Now()
	cur := int(now.Unix() % 12)
	lines := []string{
		"风吹过山岗 云掠过屋檐",
		"路灯下影子 拉得很长",
		"晚风捎来 那年的歌",
		"你笑着说过 明天见",
		"时光太匆忙 来不及道别",
		"窗台上的花 开了又谢",
		"我在等一个 不会来的夜",
		"星星都睡了 你还醒着",
		"把心事揉进 这节拍里",
		"唱到喉咙哑 也不停歇",
		"回忆像潮水 漫过堤岸",
		"天亮以前 请再唱一遍",
	}
	var buf bytes.Buffer
	buf.WriteString(`<div style="text-align:center;color:#8a8a8e;line-height:2.2;font-family:sans-serif;padding:8px 0">`)
	for i, ln := range lines {
		if i == cur {
			fmt.Fprintf(&buf, `<div style="color:#fa2c19;font-weight:700;font-size:17px">%s</div>`, ln)
		} else {
			fmt.Fprintf(&buf, `<div>%s</div>`, ln)
		}
	}
	buf.WriteString(`</div>`)
	w.Write(buf.Bytes())
}

// 模拟播放进度随时间变化，每次请求返回不同 width，验证 mp-html setData 高频刷新是否流畅
func (h *H) PlayerBar(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	now := time.Now()
	progress := int(now.Unix() % 100)
	song := fmt.Sprintf("实验曲目 #%d", int(now.Unix()/100)%10)
	fmt.Fprintf(w, `<div style="background:#1c1c1e;border-radius:16px;padding:14px;margin:12px;color:#fff;font-family:sans-serif">
  <div style="font-size:15px;font-weight:600;margin-bottom:10px">%s</div>
  <div style="background:#3a3a3c;height:6px;border-radius:3px;overflow:hidden">
    <div style="background:#fa2c19;height:100%%;width:%d%%;border-radius:3px"></div>
  </div>
  <div style="display:flex;justify-content:center;gap:24px;margin-top:12px;font-size:22px">
    <a href="action://prev" style="color:#fff;text-decoration:none">⏮</a>
    <a href="action://play-pause" style="color:#fff;text-decoration:none">⏯</a>
    <a href="action://next" style="color:#fff;text-decoration:none">⏭</a>
  </div>
</div>`, song, progress)
}

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
