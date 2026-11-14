// Package music 音乐域：B站搜索/取流/播放（本地时钟进度由 speaker 域管）。
// 全链路：入口 internal/api/music.go · hx/search.go；前端 content_search.html + player.js；表：无
package music

import (
	"strings"
	"sync"
	"time"

	"homecast/internal/service/lyric"
	"homecast/internal/bilibili"
)

// 搜索相关（MusicItem/MusicSearchResult/ParseDurationSec/Search/SearchRaw/searchRank）
// 已抽离到 search.go；歌曲过滤（IsSong/FilterSongs）在 filter.go。本文件聚焦音乐业务
// （VideoInfo 缓存 / 歌词关键词 / 取流）。

// MusicService 音乐业务
type MusicService struct {
	Client *bilibili.Client

	mu        sync.Mutex
	viewCache map[string]viewCacheEntry // bvid → view 结果（短 TTL：播放链路与歌词搜索复用，免重复打 B 站）
}

type viewCacheEntry struct {
	info *bilibili.VideoInfo
	at   time.Time
}

const viewCacheTTL = 5 * time.Minute

// VideoInfo 带缓存的 /x/web-interface/view（播放取 cid + 歌词搜索取分P标题共用）
func (m *MusicService) VideoInfo(bvid string) (*bilibili.VideoInfo, error) {
	m.mu.Lock()
	if e, ok := m.viewCache[bvid]; ok && time.Since(e.at) < viewCacheTTL {
		m.mu.Unlock()
		return e.info, nil
	}
	m.mu.Unlock()
	info, err := m.Client.GetVideoInfo(bvid)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	if m.viewCache == nil {
		m.viewCache = map[string]viewCacheEntry{}
	}
	m.viewCache[bvid] = viewCacheEntry{info: info, at: time.Now()}
	m.mu.Unlock()
	return info, nil
}

// LyricKeywords 歌词搜索关键词（对齐使用场景：播放固定第一P——前端无 cid 概念，
// 后端 VideoInfo.CID 即第一P。所以关键词 = 第一P part 清洗 + fallback 总标题兜底，
// 不做“多个分P逐个试”——第二P的词会错配给正在播的第一P）。
func (m *MusicService) LyricKeywords(bvid, fallback string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		s = lyric.CleanSearchKeyword(s)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	if m != nil && m.Client != nil && bvid != "" {
		if info, err := m.VideoInfo(bvid); err == nil {
			for _, p := range info.Pages {
				part := strings.TrimSpace(p.Part)
				if part == "" {
					continue
				}
				add(part) // 只取第一P（当前播放的P）；占位标题（如==总标题）清洗后与 fallback 去重，自然收敛
				break
			}
		}
	}
	add(fallback)
	return out
}

// GetAudioStream 播放链路：view 拿 cid → playurl 取 DASH 音频
func (m *MusicService) GetAudioStream(bvid string, quality int) (*bilibili.AudioStreamResult, error) {
	info, err := m.VideoInfo(bvid)
	if err != nil {
		return nil, err
	}
	return m.Client.GetBestAudioURL(bvid, info.CID, quality)
}
