// Package service 业务层（对齐 Python music_service / fav_service / lyric_service）
package service

import (
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

// Search 搜索 → MusicItem 列表
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
		})
	}
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
