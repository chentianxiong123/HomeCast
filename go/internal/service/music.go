// Package service 业务层（对齐 Python music_service / fav_service / lyric_service）
package service

import (
	"sort"
	"strconv"
	"strings"

	"homecast/internal/bilibili"
)

// MusicItem 播放列表条目（前端 MusicItem 对齐）
type MusicItem struct {
	BVID        string `json:"bvid"`
	Title       string `json:"title"`
	Artist      string `json:"artist"`
	Cover       string `json:"cover"`
	Duration    string `json:"duration"`
	DurationSec int    `json:"duration_sec"`
	PlayCount   int    `json:"play_count"`
	Typename    string `json:"typename"`
}

// MusicSearchResult 搜索返回
type MusicSearchResult struct {
	Total int         `json:"total"`
	List  []MusicItem `json:"list"`
}

// ParseDurationSec "3:43" / "1:02:33" → 秒（对齐 Python _parse_duration_sec）
func ParseDurationSec(s string) int {
	parts := []int{}
	for _, p := range strings.Split(s, ":") {
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0
		}
		parts = append(parts, n)
	}
	switch len(parts) {
	case 3:
		return parts[0]*3600 + parts[1]*60 + parts[2]
	case 2:
		return parts[0]*60 + parts[1]
	case 1:
		return parts[0]
	}
	return 0
}

// MusicService 音乐业务
type MusicService struct {
	Client *bilibili.Client
}

// searchRank 加权分（对齐 X11 python 版 _rank）：越低越靠前。
// 标题包装度（符号/包装词）越多越沉底；教学/碎片(<90s)/过长(>600s)降权
func searchRank(it MusicItem) int {
	pkg := 0
	for _, ch := range "[]【】（）()" {
		pkg += strings.Count(it.Title, string(ch))
	}
	low := strings.ToLower(it.Title)
	for _, w := range []string{"4k", "无损", "hi-res", "hires", "爷青回", "修复", "极致", "超清", "高清", "最高音质", "官方", "精彩", "现场"} {
		if strings.Contains(low, w) {
			pkg++
		}
	}
	if it.Typename == "音乐教学" {
		pkg += 2
	}
	if it.DurationSec > 0 && (it.DurationSec < 90 || it.DurationSec > 600) {
		pkg += 2
	}
	return pkg
}

// Search 搜索 → MusicItem 列表（rank 加权排序，同权重内短歌优先；对齐 python 版）
func (m *MusicService) Search(keyword string, page, pageSize int) (*MusicSearchResult, error) {
	res, err := m.Client.Search(keyword, page, pageSize)
	if err != nil {
		return nil, err
	}
	items := make([]MusicItem, 0, len(res.Result))
	for _, it := range res.Result {
		items = append(items, MusicItem{
			BVID:        it.BVID,
			Title:       it.Title,
			Artist:      it.Author,
			Cover:       it.Pic,
			Duration:    it.Duration,
			DurationSec: ParseDurationSec(it.Duration),
			PlayCount:   it.Play,
			Typename:    it.Typename,
		})
	}
	// 时长解析失败滤掉（对齐 python sec>0）；rank+时长升序
	filtered := items[:0]
	for _, it := range items {
		if it.DurationSec > 0 {
			filtered = append(filtered, it)
		}
	}
	items = filtered
	sort.SliceStable(items, func(i, j int) bool {
		ri, rj := searchRank(items[i]), searchRank(items[j])
		if ri != rj {
			return ri < rj
		}
		return items[i].DurationSec < items[j].DurationSec
	})
	return &MusicSearchResult{Total: res.NumResults, List: items}, nil
}

// GetAudioStream 播放链路：view 拿 cid → playurl 取 DASH 音频
func (m *MusicService) GetAudioStream(bvid string, quality int) (*bilibili.AudioStreamResult, error) {
	info, err := m.Client.GetVideoInfo(bvid)
	if err != nil {
		return nil, err
	}
	return m.Client.GetBestAudioURL(bvid, info.CID, quality)
}
