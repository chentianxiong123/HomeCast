package service

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"homecast/internal/bilibili"
)

// CleanSearchKeyword 关键词清洗：对齐 musicxx 智能识别（去 B 站标题噪音）+ bili-music 正则回退
func TestCleanSearchKeyword(t *testing.T) {
	cases := []struct{ in, want string }{
		// 《》优先提取
		{"【4K超清】周杰伦《晴天》MV", "晴天"},
		{"周杰伦「晴天」官方", "晴天"},
		{"周杰伦《晴天》杰伦作曲", "晴天"},
		// 无书名号：剥【】，去噪音词
		{"【4K】周杰伦 - 晴天 (现场版)", "周杰伦 - 晴天"},
		{"周杰伦 晴天 无损 FLAC", "周杰伦 晴天"},
		{"晴 天 1080P 纯享", "晴 天"},
		// 正常合集名（无【】无噪音词）不被误伤
		{"周杰伦50首精选", "周杰伦50首精选"},
		// 空/空白
		{"", ""},
		{"   ", ""},
		// 分隔符归一 + 分P序号前缀（合集场景）
		{"A·B", "A B"},
		{"001.周杰伦-晴天", "周杰伦-晴天"},
		{"12、夜曲", "夜曲"},
	}
	for _, c := range cases {
		got := CleanSearchKeyword(c.in)
		if got != c.want {
			t.Errorf("CleanSearchKeyword(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// 超长截断
	long := strings.Repeat("歌", 60)
	if got := CleanSearchKeyword(long); len(got) > 40 {
		t.Errorf("超长未截断: len=%d", len(got))
	}
}

// LyricKeywords 分P标题选择：播放固定第一P（前端无 cid，后端 VideoInfo.CID=第一P）→
// 只取第一个有效 part（清洗后）+ fallback 兜底（bili-music 分P语义，但收敛到当前播放的P）
func TestLyricsKeywords(t *testing.T) {
	mk := func(pages ...string) *bilibili.VideoInfo {
		info := &bilibili.VideoInfo{Title: "周杰伦50首精选"}
		for _, p := range pages {
			info.Pages = append(info.Pages, struct {
				CID  json.RawMessage `json:"cid"`
				Part string          `json:"part"`
			}{Part: p})
		}
		return info
	}
	mkMusic := func(info *bilibili.VideoInfo) *MusicService {
		m := &MusicService{Client: &bilibili.Client{}}
		m.mu.Lock()
		m.viewCache = map[string]viewCacheEntry{"BV1": {info: info, at: time.Now()}}
		m.mu.Unlock()
		return m
	}

	// 第一P是歌名 → 只取第一P（当前播放的P），不走第二P
	m := mkMusic(mk("001.周杰伦-晴天", "002.周杰伦-夜曲", "003.周杰伦-星晴"))
	if got := m.LyricKeywords("BV1", "周杰伦50首精选"); !reflect.DeepEqual(got, []string{"周杰伦-晴天", "周杰伦50首精选"}) {
		t.Errorf("LyricKeywords = %v", got)
	}
	// 第一P是占位标题（跳过后才轮到第二P——但播放仍第一P，宁可 fallback 也不错配）
	m1 := mkMusic(mk("周杰伦50首精选", "晴天"))
	if got := m1.LyricKeywords("BV1", "周杰伦50首精选"); !reflect.DeepEqual(got, []string{"周杰伦50首精选"}) {
		t.Errorf("占位第一P = %v", got)
	}
	// 视图失败/无 Client：纯 fallback 清洗
	m3 := &MusicService{}
	if got := m3.LyricKeywords("BV1", "【4K】晴天 无损"); !reflect.DeepEqual(got, []string{"晴天"}) {
		t.Errorf("无 Client 回退 = %v", got)
	}
}

// SearchLyricCandidatesByTitles 空输入直接 nil（不触发网络）
func TestSearchLyricCandidatesByTitlesEmpty(t *testing.T) {
	if got := SearchLyricCandidatesByTitles(nil, 8); got != nil {
		t.Errorf("空标题应 nil: %v", got)
	}
	if got := SearchLyricCandidatesByTitles([]string{"", "  "}, 8); got != nil {
		t.Errorf("空白标题应 nil: %v", got)
	}
}