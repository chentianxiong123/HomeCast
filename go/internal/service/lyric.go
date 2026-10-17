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

// LyricTable lyric_sels 表名
const LyricTable = "lyric_sels"

// LyricSchema lyric_sels 表定义（表结构归业务域；建表执行在 store.Migrate 统一跑）
const LyricSchema = `CREATE TABLE IF NOT EXISTS lyric_sels (
	bvid TEXT PRIMARY KEY, sid INTEGER NOT NULL DEFAULT 0,
	name TEXT DEFAULT '', artist TEXT DEFAULT '', cover TEXT DEFAULT '',
	duration INTEGER DEFAULT 0, offset REAL DEFAULT 0,
	updated_at INTEGER DEFAULT 0)`

const (
	lyricSearchURL = "https://music.163.com/api/search/get/web"
	lyricFetchURL  = "https://music.163.com/api/song/lyric"
)

var lyricSkipPrefixes = []string{
	"作词", "作曲", "编曲", "制作", "录音", "混音", "监制",
	"OP:", "SP:", "和声", "配唱",
}

var lrcTimeRe = regexp.MustCompile(`\[(\d+):(\d+(?:.\d+)?)\]`)

// offsetTagRe LRC/增强歌词内嵌的整体偏移标签：`[offset:-500]` / `[offset,-500]`
// 语义对齐 bili-music（desktop_lyrics_controller）：标签值负=歌词整体提前。
// 解析时折进每行时间（t' = t + offsetMs/1000），前端零感知。
var offsetTagRe = regexp.MustCompile(`(?i)^\[offset[:,](-?\d+)\]$`)

// ── 搜索关键词清洗（musicxx 智能识别规则子集 + bili-music 正则回退同款） ──
// 对齐 bili-music meting_logic.extractSearchKeyword：《》「」优先提取，剥【】；
// 再按 musicxx 智能识别思路去 B 站标题噪音词（4K/无损/纯享…）：
// B 站标题混冗余是歌词匹配错的第一大原因（musicxx 文档原话）。
var (
	bookTitleRe = regexp.MustCompile(`[《「〈]([^》」〉]+)[》」〉]`)
	bracketRe   = regexp.MustCompile(`[【\[(][^】\])]*[】\])]`)
	sepRe       = regexp.MustCompile(`[·•|/\\]`)
	seqPrefixRe = regexp.MustCompile(`^\s*\d+[.、．]\s*`) // 分P序号前缀：“001.周杰伦-晴天”→“周杰伦-晴天”
	// 画质/格式/版本噪音词（不杀“翻唱/现场/Live”——那是版本信息，musicxx 归为版本字段权重，非删）
	noiseWords = []string{"1080P", "4K", "720P", "超清", "高清", "无损", "FLAC", "HIRES", "HQ", "MV", "官方", "哔哩哔哩", "BILI", "纯享", "完整版", "现场版"}
)

// CleanSearchKeyword 搜索关键词清洗：歌名优先（《》内），去噪音词，折叠空白。
func CleanSearchKeyword(title string) string {
	s := strings.TrimSpace(title)
	if s == "" {
		return ""
	}
	if m := bookTitleRe.FindStringSubmatch(s); m != nil {
		return strings.TrimSpace(m[1])
	}
	s = bracketRe.ReplaceAllString(s, " ")
	up := strings.ToUpper(s)
	for _, w := range noiseWords {
		up = strings.ReplaceAll(up, w, " ")
	}
	s = sepRe.ReplaceAllString(up, " ")
	s = seqPrefixRe.ReplaceAllString(s, "")
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 40 {
		s = s[:40] // 防超长关键词
	}
	return strings.TrimSpace(s)
}

// timedLineRe 增强格式行：`[startMs,durMs](字级时间戳…) 文本`（yrc/qrc/krc）
var timedLineRe = regexp.MustCompile(`^\[(\d+),(\d+)(?:,\d+)?\]\s*(.*)$`)

// timedWordRe 字级时间戳 `(ms,ms,dur)` / `<ms,ms>`（逐字卡拉OK），解析时剥离
var timedWordRe = regexp.MustCompile(`(?:\(-?\d+,-?\d+(?:,-?\d+)?\)|<-?\d+,-?\d+(?:,-?\d+)?>)`)

// LyricLine [秒, 行, 行时长秒]（前端 LyricPanel 用 l[0]/l[1]，l[2] 行时长：0=未知，前端用下一行差估算进度）
type LyricLine [3]any

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

// parseLRC LRC 文本 → [][秒,行,时长]，时间升序；过滤元信息行；识别 [offset:] 标签折入行时间；无词返回 nil
func parseLRC(text string) []LyricLine {
	var lines []LyricLine
	offsetMs := 0
	for _, raw := range strings.Split(text, "\n") {
		ln := strings.TrimSpace(raw)
		if m := offsetTagRe.FindStringSubmatch(ln); m != nil {
			offsetMs, _ = strconv.Atoi(m[1]) // 文件内整体偏移（bili-music 同款语义）
			continue
		}
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
		lines = append(lines, LyricLine{t + float64(offsetMs)/1000, txt, 0.0})
	}
	sort.Slice(lines, func(i, j int) bool {
		return lines[i][0].(float64) < lines[j][0].(float64)
	})
	if len(lines) == 0 {
		return nil
	}
	return lines
}

// parseYRC 增强格式（yrc 逐字卡拉OK / qrc / krc）：`[start,dur](字级…) 文本`
// 行时长是自带的（LRC 没有，只能靠下一行差值估算）→ 逐行进度直接用；剥离字级时间戳；过滤元信息行
func parseYRC(text string) []LyricLine {
	var lines []LyricLine
	offsetMs := 0
	for _, raw := range strings.Split(text, "\n") {
		ln := strings.TrimSpace(raw)
		if m := offsetTagRe.FindStringSubmatch(ln); m != nil {
			offsetMs, _ = strconv.Atoi(m[1])
			continue
		}
		m := timedLineRe.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		start, _ := strconv.Atoi(m[1])
		dur, _ := strconv.Atoi(m[2])
		txt := strings.TrimSpace(timedWordRe.ReplaceAllString(m[3], ""))
		if txt == "" || hasPrefixAny(txt, lyricSkipPrefixes) {
			continue
		}
		lines = append(lines, LyricLine{
			float64(start+offsetMs) / 1000,
			txt,
			float64(dur) / 1000,
		})
	}
	sort.Slice(lines, func(i, j int) bool {
		return lines[i][0].(float64) < lines[j][0].(float64)
	})
	if len(lines) == 0 {
		return nil
	}
	return lines
}

// parseLyrics 自动检测：行首 `[数字,数字]`（增强格式 yrc/qrc/krc）→ 逐字解析；否则 LRC
func parseLyrics(text string) []LyricLine {
	for _, raw := range strings.Split(text, "\n") {
		ln := strings.TrimSpace(raw)
		if ln == "" {
			continue
		}
		if timedLineRe.MatchString(ln) {
			return parseYRC(text)
		}
		if lrcTimeRe.MatchString(ln) {
			break // LRC，走 parseLRC
		}
	}
	return parseLRC(text)
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

// fetchLyricRaw 网易云歌词原始文本（sid=歌曲id）→ (lrc, yrc)
// lv/kv/tv/rv/yv 全开：lrc=主歌词，yrc=逐字卡拉OK（增强格式，带行时长），tlyric=翻译（tv=1 不再主动关）
// 之前 tv=-1 只取 lrc，逐字/翻译白丢（bili-music 同款语义：有 karaokeLyric 优先）
func fetchLyricRaw(sid int) (lrcText, yrcText string) {
	params := url.Values{}
	params.Set("id", strconv.Itoa(sid))
	params.Set("lv", "1")
	params.Set("kv", "1")
	params.Set("tv", "1")
	params.Set("rv", "1")
	params.Set("yv", "1")
	j, err := neteaseGet(lyricFetchURL + "?" + params.Encode())
	if err != nil {
		return
	}
	if code, _ := j["code"].(float64); int(code) == 200 {
		if lrc, ok := j["lrc"].(map[string]any); ok {
			lrcText, _ = lrc["lyric"].(string)
		}
		if yrc, ok := j["yrc"].(map[string]any); ok {
			yrcText, _ = yrc["lyric"].(string)
		}
	}
	return
}

// fetchLyricLines sid → 歌词行（优先 yrc 逐字版，其次 lrc；[offset:] 标签已折入行时间）
func fetchLyricLines(sid int) []LyricLine {
	lrcT, yrcT := fetchLyricRaw(sid)
	if yrcT != "" {
		if l := parseLyrics(yrcT); l != nil {
			return l
		}
	}
	if lrcT != "" {
		return parseLyrics(lrcT)
	}
	return nil
}

// SearchLyricCandidates 搜歌名 → 候选。惰性取词（musicxx 两段式 API 同思路）：
// 搜索只回元数据；第一个有词候选验证并带 Lines（保住“自动选第一有词源、自动跳脏候选”语义），
// 其余候选不拉正文（前端选中时懒拉 /api/v1/music/lyric?sid=）→ 9 次串行请求降到 2 次。
func SearchLyricCandidates(keyword string, limit int) []LyricResult {
	keyword = CleanSearchKeyword(keyword)
	if keyword == "" {
		return nil
	}
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
	gotLines := false
	for _, s := range songs {
		song, ok := s.(map[string]any)
		if !ok {
			continue
		}
		sid := int(toFloat(song["id"]))
		if sid == 0 {
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
		r := LyricResult{
			ID:       sid,
			Name:     strAny(song["name"]),
			Artist:   artist,
			Album:    album,
			Duration: int(toFloat(song["duration"]) / 1000),
		}
		if !gotLines {
			if lines := fetchLyricLines(sid); len(lines) >= 3 {
				r.Lines = lines
				gotLines = true
			} else {
				continue // 无词/拉词失败：不展示（保持“候选只含有词≥3行”体验）
			}
		}
		out = append(out, r)
	}
	return out
}

// GetLyricLines 按备选关键词取第一源；或 sid 精确取。找不到返回 nil（业务正常）
func GetLyricLines(keywords []string, sid int) *LyricResult {
	if len(keywords) == 0 && sid == 0 {
		return nil // 空关键词不发网易云（挂件 bvid 模式无已选源时）
	}
	if sid > 0 {
		lines := fetchLyricLines(sid)
		if lines != nil {
			return &LyricResult{ID: sid, Lines: lines}
		}
		return nil
	}
	cands := SearchLyricCandidatesByTitles(keywords, 8)
	if len(cands) > 0 && len(cands[0].Lines) >= 3 {
		return &cands[0]
	}
	return nil
}

// SearchLyricCandidatesByTitles 备用标题逐个搜（bili-music lyricSearchTitles 同款）：
// 第一个搜到候选（且第一候选有词）的关键词即用，避免“第一P占位标题”直接翻车。
func SearchLyricCandidatesByTitles(titles []string, limit int) []LyricResult {
	for _, kw := range titles {
		if kw == "" {
			continue
		}
		if res := SearchLyricCandidates(kw, limit); len(res) > 0 {
			return res
		}
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
