package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
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
	Offset   float64     `json:"offset,omitempty"` // 本歌已存档的对齐偏移（秒，bvid 模式返回）
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

// ── 已选歌词源记忆（持久化：SQLite lyric_sels 表） ──
// 对齐 music.py 的 LYRIC_MAP（bvid → 已选歌词版本 {id,name,artist,dur,offset}）：
// 选的是哪个候选 + 手动对齐偏移 ±0.5s，都是用户数据，随增删改查持久化，不是缓存。
// 歌源正文缓存（前端 localStorage LRU）与这里互不干扰：缓存负责“秒显歌词行”，
// 这里负责“记住选了什么、调了多少”。

// LyricSel 一条已选歌词记录（bvid → 候选源 + 对齐偏移）
type LyricSel struct {
	BVID     string
	SID      int
	Name     string
	Artist   string
	Cover    string
	Duration int
	Offset   float64
}

// LyricStore SQLite 持久化 + 内存兜底（db==nil 时退化为内存 map，测试/无库环境用）
type LyricStore struct {
	db  *sql.DB
	mu  sync.Mutex
	mem map[string]LyricSel
}

// NewLyricStore 构造；db 可 nil（纯内存模式）
func NewLyricStore(db *sql.DB) *LyricStore {
	return &LyricStore{db: db, mem: map[string]LyricSel{}}
}

// SetSelection 记录/更新歌曲的已选歌词源（sid<=0 清除记录）
func (s *LyricStore) SetSelection(bvid string, sel LyricSel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if bvid == "" {
		return
	}
	if sel.SID <= 0 {
		delete(s.mem, bvid)
		if s.db != nil {
			_, _ = s.db.Exec(`DELETE FROM lyric_sels WHERE bvid = ?`, bvid)
		}
		return
	}
	s.mem[bvid] = sel
	if s.db != nil {
		_, _ = s.db.Exec(`INSERT INTO lyric_sels (bvid,sid,name,artist,cover,duration,offset,updated_at)
			VALUES (?,?,?,?,?,?,?,?) ON CONFLICT(bvid) DO UPDATE SET
			sid=excluded.sid, name=excluded.name, artist=excluded.artist, cover=excluded.cover,
			duration=excluded.duration, updated_at=excluded.updated_at`,
			sel.BVID, sel.SID, sel.Name, sel.Artist, sel.Cover, sel.Duration, sel.Offset, time.Now().Unix())
	}
}

// SelectedSID 返回 bvid 的已选歌词源 sid（无则 0）
func (s *LyricStore) SelectedSID(bvid string) int {
	v, _ := s.Get(bvid)
	return v.SID
}

// Get 返回 bvid 的完整记录（无则零值 + false）
func (s *LyricStore) Get(bvid string) (LyricSel, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if bvid == "" {
		return LyricSel{}, false
	}
	if s.db == nil {
		v, ok := s.mem[bvid]
		return v, ok
	}
	var v LyricSel
	err := s.db.QueryRow(`SELECT bvid,sid,name,artist,cover,duration,offset FROM lyric_sels WHERE bvid = ?`, bvid).
		Scan(&v.BVID, &v.SID, &v.Name, &v.Artist, &v.Cover, &v.Duration, &v.Offset)
	if err != nil {
		return LyricSel{}, false
	}
	return v, true
}

// SetOffset 调整歌曲的歌词对齐偏移（无记录时建一条空源记录，只记偏移）
func (s *LyricStore) SetOffset(bvid string, offset float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if bvid == "" {
		return
	}
	old := s.mem[bvid]
	old.BVID = bvid
	old.Offset = offset
	s.mem[bvid] = old
	if s.db != nil {
		_, _ = s.db.Exec(`INSERT INTO lyric_sels (bvid,sid,name,artist,cover,duration,offset,updated_at)
			VALUES (?,0,'','','',0,?,?) ON CONFLICT(bvid) DO UPDATE SET offset=excluded.offset, updated_at=excluded.updated_at`,
			bvid, offset, time.Now().Unix())
	}
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
