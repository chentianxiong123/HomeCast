package lyric

import (
	"strings"
	"testing"
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

// SearchLyricCandidatesByTitles 空输入直接 nil（不触发网络）
func TestSearchLyricCandidatesByTitlesEmpty(t *testing.T) {
	if got := SearchLyricCandidatesByTitles(nil, 8); got != nil {
		t.Errorf("空标题应 nil: %v", got)
	}
	if got := SearchLyricCandidatesByTitles([]string{"", "  "}, 8); got != nil {
		t.Errorf("空白标题应 nil: %v", got)
	}
}