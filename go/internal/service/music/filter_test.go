package music

import "testing"

func TestIsSong(t *testing.T) {
	cases := []struct {
		name     string
		typename string
		title    string
		sec      int
		want     bool
	}{
		// 该留的原曲/音源
		{"原曲", "音乐综合", "晴天", 270, true},
		{"Hi-Res包装", "音乐综合", "【Hi-Res无损】晴天-周杰伦", 268, true},
		{"官方MV", "MV", "周杰伦-晴天 官方MV", 270, true},
		{"无时长不强判", "音乐综合", "晴天", 0, true},
		// 分区黑名单
		{"翻唱", "翻唱", "晴天 翻唱", 270, false},
		{"电台", "电台", "晴天", 270, false},
		{"乐评盘点", "乐评盘点", "十年后再听晴天", 600, false},
		{"音乐教学", "音乐教学", "晴天吉他教学", 600, false},
		// 标题黑名单（含伪装在音乐综合里的）
		{"伴奏", "音乐综合", "晴天 纯伴奏", 270, false},
		{"英文cover", "音乐综合", "晴天 cover", 270, false},
		{"钢琴版", "音乐综合", "晴天 钢琴版", 270, false},
		{"降调", "音乐综合", "晴天 0.8降调版", 270, false},
		{"remix", "音乐综合", "晴天 Remix", 270, false},
		{"解说", "音乐综合", "你根本没听懂晴天", 300, false},
		{"卡点片段", "音乐综合", "晴天 卡点", 30, false},
		{"串烧合集", "音乐综合", "周杰伦串烧", 800, false},
		// 时长
		{"过短", "音乐综合", "晴天片段", 30, false},
		{"过长合集", "音乐综合", "晴天", 1000, false},
	}
	for _, c := range cases {
		if got := IsSong(c.typename, c.title, c.sec); got != c.want {
			t.Errorf("IsSong(%q,%q,%d)=%v, want %v", c.typename, c.title, c.sec, got, c.want)
		}
	}
}

func TestFilterSongs(t *testing.T) {
	items := []MusicItem{
		{BVID: "BV1", Title: "晴天", Typename: "音乐综合", DurationSec: 270},
		{BVID: "BV2", Title: "晴天 纯伴奏", Typename: "音乐综合", DurationSec: 270}, // 标题踢
		{BVID: "BV3", Title: "晴天 翻唱", Typename: "翻唱", DurationSec: 270},       // 分区踢
		{BVID: "BV4", Title: "晴天片段", Typename: "音乐综合", DurationSec: 30},         // 时长踢
		{BVID: "BV5", Title: "稻香", Typename: "音乐综合", DurationSec: 224},
	}
	// 开过滤：留 BV1/BV5
	got := FilterSongs(items, true)
	if len(got) != 2 || got[0].BVID != "BV1" || got[1].BVID != "BV5" {
		t.Errorf("FilterSongs(on)=%v, want [BV1 BV5]", bvids(got))
	}
	// 关过滤：原样全留
	all := FilterSongs(items, false)
	if len(all) != 5 {
		t.Errorf("FilterSongs(off)=%d条, want 5", len(all))
	}
}

func bvids(items []MusicItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.BVID
	}
	return out
}
