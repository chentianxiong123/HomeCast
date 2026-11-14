package music

import "strings"

// 歌曲过滤器：搜索结果里混入大量"不是歌"的视频（cover/伴奏/翻唱/电台/乐评/教学/合集/卡点），
// 听歌场景要剔除。三路信号（标题关键词 + 分区 typename + 时长）硬排除，宁缺毋滥。
//
// 设计取舍：
//   - 标题 + 分区是主力（B站自己打的 typename 标签最准；标题关键词兜住"音乐综合"分区里伪装成歌的杂质）
//   - 时长是辅助（歌曲合理区间；<90s 多为卡点/试听片段，>15min 多为合集/串烧/电台长录）
//   - 不用播放量门槛（用户明确：播放量不代表是不是歌；搬运的原曲播放量低也是歌）
//   - FilterSongs(filterOn=false) 关闭 → 原样返回（用户想搜 MV 合集/伴奏/教学时可关）

// badTypenames 分区黑名单：B站 search 返回的 typename，这些分区默认不是"一首歌"。
// 保留：音乐综合（原曲/Hi-Res 音源多在此）、MV（官方原曲 MV）、音乐现场、音乐单曲、音乐合集(演出版)。
var badTypenames = map[string]bool{
	"翻唱":    true, // 别人唱的，非原唱
	"电台":    true, // 电台/朗读/伴奏常挂这
	"乐评盘点":  true, // 乐评/盘点/解说
	"音乐教学":  true, // 教学/教程
	"音乐文化":  true, // 文化科普
	"音乐节目":  true, // 综艺/节目
	"音乐其他":  true,
	"VOCALOID·UTAU": true, // 虚拟歌手投稿（调教/原创曲，非原曲）
}

// badTitleWords 标题黑名单：出现在标题里就判非歌（不分大小写匹配）。
// 覆盖：翻唱/伴奏/演奏/变速/教学/盘点/卡点/试听/串烧等非"原曲音源"形态。
// 注意：不屏蔽 4k/无损/Hi-Res/修复 等——那些是原曲的高质音源包装，是想要的。
var badTitleWords = []string{
	// 翻唱/演奏（别人重新演绎，非原唱音源）
	"cover", "翻唱", "翻弹", "指弹", "弹唱", "钢琴版", "吉他版", "纯音乐版", "演奏",
	"remix", "remake", "bootleg", "flip",
	// 伴奏/卡拉OK（无人声）
	"伴奏", "inst", "instrumental", "纯伴奏", "和声伴奏", "karaoke", "k歌", "ktv", "伴唱",
	"off vocal", "backing track",
	// 变速变调
	"降调", "升调", "变速", "0.8", "1.2倍", "慢速", "加速", "slowed", "sped up", "reverb", "8d", "8d环绕",
	// 教学/乐评/盘点/反应/解析
	"教学", "教程", "怎么唱", "如何唱", "评测", "乐评", "盘点", "解说", "解析", "细听", "听懂", "赏析",
	"reaction", "反应", "一起听", "带你听", "讲了", "分析",
	// 合集/串烧/串唱/medley（多首拼，非单曲）
	"串烧", "合集", "medley", "串唱", "连唱", "大赏", "全曲", "全集", "top10", "排行", "歌单",
	// 卡点/试听/片段/铃声（碎片，非完整歌）
	"卡点", "踩点", "试听", "片段", "高潮", "副歌", "铃声", "剪辑", "cut", "片段",
	// 鬼畜/MAD/AMV（二次创作）
	"鬼畜", "mad", "amv", "remix向",
	// 电台/助眠/白噪音（长录，非歌）
	"电台", "助眠", "白噪音", "白噪声", "asmr", "睡眠", "放松",
}

// IsSong 判定一个搜索结果是不是"一首歌"（用于听歌场景过滤）。
// 返回 false = 判定非歌，应剔除。三信号任一命中即剔除（宁缺毋滥）。
func IsSong(typename, title string, durationSec int) bool {
	// 1) 分区黑名单（最准：B站自打的标签）
	if typename != "" && badTypenames[typename] {
		return false
	}
	// 2) 标题黑名单（兜住"音乐综合"里伪装成歌的 cover/伴奏/解说）
	low := strings.ToLower(title)
	for _, w := range badTitleWords {
		if strings.Contains(low, strings.ToLower(w)) {
			return false
		}
	}
	// 3) 时长区间（歌曲合理范围；0=解析失败不强判，交给调用方）
	if durationSec > 0 {
		if durationSec < 90 || durationSec > 900 { // <1.5min 片段 / >15min 合集电台
			return false
		}
	}
	return true
}

// FilterSongs 就地过滤掉非歌条目（保持相对顺序）。filterOn=false → 原样保留（过滤开关）。
func FilterSongs(items []MusicItem, filterOn bool) []MusicItem {
	if !filterOn {
		return items
	}
	out := items[:0]
	for _, it := range items {
		if IsSong(it.Typename, it.Title, it.DurationSec) {
			out = append(out, it)
		}
	}
	return out
}
