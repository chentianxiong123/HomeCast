package server

import (
	"homecast/internal/api"
	"homecast/internal/service/lyric"
	"homecast/internal/service/widget"
)

// lyricSource 适配器：把"bvid → 歌词行 + offset"封装成 widget.LyricSource。
// 与 /music/lyric?bvid= 同一份歌词：已选源（SelectedSID）优先 + bvid 模式带 offset；
// 无已选源时按歌名关键词自动选第一有词源（不联网，用前端上报的 title）。
type lyricSource struct {
	handler *api.LyricHandler
	state   *widget.WidgetState
}

// Lines 实现 widget.LyricSource：按 bvid 拉一次歌词行 + 对齐 offset（切歌低频调用）。
func (l *lyricSource) Lines(bvid string) ([][3]any, float64, bool) {
	if bvid == "" {
		return nil, 0, false
	}
	// ① 已选源优先（用户切源上报持久化）：与 /music/lyric?bvid= 同一份歌词 + offset
	if sel := l.handler.Lyric.SelectedSID(bvid); sel > 0 {
		if res := lyric.GetLyricLines(nil, sel); res != nil {
			if selRec, ok := l.handler.Lyric.Get(bvid); ok {
				return toLrcLines(res.Lines), selRec.Offset, true
			}
			return toLrcLines(res.Lines), 0, true
		}
	}
	// ② 无已选源：用前端上报的歌名（WidgetState.Title）关键词自动选第一有词源
	title := ""
	if l.state != nil {
		_, _, title, _, _, _ = l.state.Get()
	}
	if title != "" {
		if kws := lyric.CleanSearchKeyword(title); kws != "" {
			if res := lyric.GetLyricLines([]string{kws}, 0); res != nil {
				return toLrcLines(res.Lines), 0, true
			}
		}
	}
	return nil, 0, false
}

// toLrcLines 转 lyric.LyricLine（[3]any）→ [][3]any（widget.LyricSource 行格式）
func toLrcLines(in []lyric.LyricLine) [][3]any {
	out := make([][3]any, 0, len(in))
	for _, x := range in {
		out = append(out, x)
	}
	return out
}

// newLyricSource 构造（注入到 widget.NewWidgetState）
func newLyricSource(h *api.LyricHandler, state *widget.WidgetState) *lyricSource {
	return &lyricSource{handler: h, state: state}
}

var _ widget.LyricSource = (*lyricSource)(nil)