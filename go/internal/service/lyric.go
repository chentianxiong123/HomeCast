package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ── 歌词（对齐桌面播放器：网易云免费源，无需登录/cookie） ──

const (
	lyricSearchURL = "https://music.163.com/api/search/get/web"
	lyricFetchURL  = "https://music.163.com/api/song/lyric"
)

var lyricSkipPrefixes = []string{
	"作词", "作曲", "编曲", "制作", "录音", "混音", "监制",
	"OP:", "SP:", "和声", "配唱",
}

var lrcTimeRe = regexp.MustCompile(`\[(\d+):(\d+(?:\.\d+)?)\]`)

// LyricLine [秒, 行]（前端 LyricPanel 用 l[0]/l[1]）
type LyricLine [2]any

// LyricResult 歌词返回（对齐 Python get_lyric_lines）
type LyricResult struct {
	ID       int         `json:"id"`
	Name     string      `json:"name"`
	Artist   string      `json:"artist"`
	Album    string      `json:"album"`
	Duration int         `json:"duration"` // 秒（候选行展示 mm:ss）
	Lines    []LyricLine `json:"lines"`
}

// parseLRC LRC 文本 → [][秒,行]，时间升序；过滤元信息行；无词返回 nil
func parseLRC(text string) []LyricLine {
	var lines []LyricLine
	for _, raw := range strings.Split(text, "\n") {
		ln := strings.TrimSpace(raw)
		m := lrcTimeRe.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		mm, _ := strconv.Atoi(m[1])
		ss, _ := strconv.ParseFloat(m[2], 64)
		t := float64(mm)*60 + ss
		txt := strings.TrimSpace(ln[len(m[0]):])
		if txt == "" || hasPrefixAny(txt, lyricSkipPrefixes) {
			continue
		}
		lines = append(lines, LyricLine{t, txt})
	}
	sort.Slice(lines, func(i, j int) bool {
		return lines[i][0].(float64) < lines[j][0].(float64)
	})
	if len(lines) == 0 {
		return nil
	}
	return lines
}

func hasPrefixAny(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

var neteaseHC = &http.Client{Timeout: 15 * time.Second}

func neteaseGet(u string) (map[string]any, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://music.163.com/")
	resp, err := neteaseHC.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var j map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&j); err != nil {
		return nil, err
	}
	return j, nil
}

// fetchLyric 网易云 LRC 文本（sid=歌曲id）；无词返回 ""
func fetchLyric(sid int) string {
	params := url.Values{}
	params.Set("id", strconv.Itoa(sid))
	params.Set("lv", "1")
	params.Set("kv", "1")
	params.Set("tv", "-1")
	j, err := neteaseGet(lyricFetchURL + "?" + params.Encode())
	if err != nil {
		return ""
	}
	if code, _ := j["code"].(float64); int(code) == 200 {
		if lrc, ok := j["lrc"].(map[string]any); ok {
			if s, ok := lrc["lyric"].(string); ok {
				return s
			}
		}
	}
	return ""
}

// SearchLyricCandidates 搜歌名 → 候选（只含有词 ≥3 行）
func SearchLyricCandidates(keyword string, limit int) []LyricResult {
	if limit <= 0 || limit > 20 {
		limit = 8
	}
	params := url.Values{}
	params.Set("s", keyword)
	params.Set("type", "1")
	params.Set("limit", strconv.Itoa(limit))
	params.Set("offset", "0")
	j, err := neteaseGet(lyricSearchURL + "?" + params.Encode())
	if err != nil {
		return nil
	}
	result, _ := j["result"].(map[string]any)
	songs, _ := result["songs"].([]any)
	var out []LyricResult
	for _, s := range songs {
		song, ok := s.(map[string]any)
		if !ok {
			continue
		}
		sid := int(toFloat(song["id"]))
		if sid == 0 {
			continue
		}
		text := fetchLyric(sid)
		lines := parseLRC(text)
		if len(lines) < 3 {
			continue
		}
		artists, _ := song["artists"].([]any)
		artist := ""
		if len(artists) > 0 {
			if a, ok := artists[0].(map[string]any); ok {
				artist, _ = a["name"].(string)
			}
		}
		album := ""
		if al, ok := song["album"].(map[string]any); ok {
			album, _ = al["name"].(string)
		}
		out = append(out, LyricResult{
			ID:       sid,
			Name:     strAny(song["name"]),
			Artist:   artist,
			Album:    album,
			Duration: int(toFloat(song["duration"]) / 1000),
			Lines:    lines,
		})
	}
	return out
}

// GetLyricLines 按歌名自动取第一源；或 sid 精确取。找不到返回 nil（业务正常）
func GetLyricLines(keyword string, sid int) *LyricResult {
	if sid > 0 {
		text := fetchLyric(sid)
		lines := parseLRC(text)
		if lines != nil {
			return &LyricResult{ID: sid, Lines: lines}
		}
		return nil
	}
	cands := SearchLyricCandidates(keyword, 8)
	if len(cands) > 0 {
		return &cands[0]
	}
	return nil
}

// ── 小工具 ──

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return 0
}

func strAny(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}
