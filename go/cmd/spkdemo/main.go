// spkdemo —— 独立 mock：给真音箱喂【稳定不断的流】，内容随控制变。
//
// 核心模式（真机实测铁证 2026-11-13）：
//   - 音箱最稳 = 200 全量：一次拉完整文件，自己平稳播（真实 mp3 实测拉 5 次完整文件，零断流）
//   - 控制通道 = 云 play_url 下指令 → 音箱重新拉新 URL（初始播放/切歌/断流重连都走云，文档第九节）
//   - 每次操作 = 服务器按新状态 serve 全量段 + 投送带时间戳的新 URL → 音箱重拉新内容
//   - seek    = serve 从 seek 位置起的完整剩余段（无需 ffmpeg，Content-Length=剩余）
//   - 暂停    = 投送静音 mp3（120s 静音源，播着不响，验证过 0 断流）
//   - 继续    = 重投当前歌（从当前位置）
//   - 切歌    = 换源全量投
//   - 音量锁低（用户痛恨拉满）
//
// 用法：
//
//	/tmp/spkdemo                                # 内置两首正弦波 WAV（可切）
//	/tmp/spkdemo -audio /path/a.mp3 -dur 179    # 单文件
//	/tmp/spkdemo -songs "歌1.mp3:180,歌2.mp3:120"  # 多首（逗号分隔，冒号时长）
//	/tmp/spkdemo -hc http://127.0.0.1:28976 -did 108654953   # 联动真音箱
//
// 网页：http://127.0.0.1:29111
package main

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed web/index.html
var pageHTML string

var (
	flagPort  = flag.String("port", ":29111", "监听端口")
	flagAudio = flag.String("audio", "", "单个音频文件（读入内存；空=内置两首正弦波）")
	flagSongs = flag.String("songs", "", "多首：\"路径:秒,路径:秒\"（推荐）")
	flagBit   = flag.Int("bitrate", 128, "码率 kbps（seek 换算）")
	flagHC    = flag.String("hc", "http://127.0.0.1:28976", "hcserver 地址")
	flagDid   = flag.String("did", "108654953", "目标音箱 did")
	flagLanIP = flag.String("lanip", "", "本机局域网 IP（空=自动探测）")
	flagVol   = flag.Int("vol", 5, "投送前锁的音量（1-100）")
	// 静音源文件（暂停用），不存在则暂停仍投当前歌但断流
	flagSil = flag.String("sil", "/tmp/sil120.mp3", "120s 静音 mp3（暂停时投）")
)

// ---------- 音频源 ----------

type src struct {
	ID     int     `json:"id"`
	Title  string  `json:"title"`
	DurSec float64 `json:"dur"`
	Header []byte  `json:"-"` // 文件头（WAV 44B；mp3 可空）
	PCM    []byte  `json:"-"` // 音频数据（整首）
	Mime   string  `json:"-"`
	Bps    int64   `json:"-"` // 字节/秒
}

var (
	mu        sync.Mutex
	srcs      []*src
	silData   []byte // 静音文件全文（暂停时 serve）
	silMime   string
	silDur    float64
)

const sampleRate = 22050

func genWAV(freq float64, durSec int) (hdr, pcm []byte) {
	samples := sampleRate * durSec
	dataSize := samples * 2
	buf := make([]byte, 44+dataSize)
	copy(buf[0:], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:], uint32(36+dataSize))
	copy(buf[8:], "WAVE")
	copy(buf[12:], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:], 16)
	binary.LittleEndian.PutUint16(buf[20:], 1)
	binary.LittleEndian.PutUint16(buf[22:], 1)
	binary.LittleEndian.PutUint32(buf[24:], sampleRate)
	binary.LittleEndian.PutUint32(buf[28:], sampleRate*2)
	binary.LittleEndian.PutUint16(buf[32:], 2)
	binary.LittleEndian.PutUint16(buf[34:], 16)
	copy(buf[36:], "data")
	binary.LittleEndian.PutUint32(buf[40:], uint32(dataSize))
	for i := 0; i < samples; i++ {
		v := int16(math.Sin(2*math.Pi*freq*float64(i)/sampleRate) * 8000)
		binary.LittleEndian.PutUint16(buf[44+i*2:], uint16(v))
	}
	return buf[:44], buf[44:]
}

// ---------- 播放状态 ----------

type psState struct {
	cur      int     // 当前源 ID
	pcmPos   int64   // 歌内位置（秒→字节；seek 改它）
	lastOp   string
	lastOpAt time.Time
	lastLat  int64
	pushes   int64
	lastPushTime time.Time
}

var st = &psState{cur: 0, lastOpAt: time.Now()}

func curSrc() *src {
	mu.Lock()
	defer mu.Unlock()
	if st.cur < 0 || st.cur >= len(srcs) {
		return srcs[0]
	}
	return srcs[st.cur]
}

func curBPS() int64 {
	s := curSrc()
	if s.Bps > 0 {
		return s.Bps
	}
	return int64(*flagBit * 1000 / 8)
}

func curDur() float64 { return curSrc().DurSec }

// ---------- HTTP serve（全量 200，支持 start 秒参数） ----------

// handleStream：serve 全量段。?start=N → 从第 N 秒字节起返回剩余全部（200 + 真实 Content-Length）。
// 暂停态（?sil=1）→ serve 静音文件全文。
func handleStream(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("sil") == "1" && len(silData) > 0 {
		w.Header().Set("Content-Type", silMime)
		w.Header().Set("Cache-Control", "no-store, no-cache")
		w.Header().Set("Content-Length", strconv.Itoa(len(silData)))
		w.WriteHeader(http.StatusOK)
		w.Write(silData)
		log.Printf("[stream-sil] did=%s 静音 %dB", q.Get("did"), len(silData))
		return
	}
	mu.Lock()
	s := curSrc()
	startSec, _ := strconv.ParseFloat(q.Get("start"), 64)
	mu.Unlock()

	hdr := s.Header
	pcm := s.PCM
	bps := s.Bps
	var startOff int64
	if startSec > 0 {
		startOff = int64(startSec * float64(bps))
		if startOff > int64(len(pcm)) {
			startOff = int64(len(pcm))
		}
	}
	// 组装剩余全量
	body := make([]byte, 0, len(hdr)+len(pcm)-int(startOff))
	if startOff == 0 {
		body = append(body, hdr...)
	}
	if startOff < int64(len(pcm)) {
		body = append(body, pcm[startOff:]...)
	}
	if len(pcm) > 0 && startOff >= int64(len(pcm)) && len(body) == 0 {
		body = append(body, hdr...)
	}
	w.Header().Set("Content-Type", s.Mime)
	w.Header().Set("Cache-Control", "no-store, no-cache")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	w.Write(body)
	log.Printf("[stream] did=%s start=%.1fs（%dB）曲%d", q.Get("did"), startSec, len(body), st.cur)
}

// ---------- 控制 ----------

// handleControl：每次操作 → 更新状态 → 按新状态投新 URL（带时间戳）→ 音箱重新拉
func handleControl(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Op  string  `json:"op"`
		Sec float64 `json:"sec"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	mu.Lock()
	switch body.Op {
	case "play":
		if st.pcmPos >= int64(len(curSrc().PCM)) {
			st.pcmPos = 0
		}
		st.lastOp = "play"
		st.lastOpAt = time.Now()
	case "pause":
		st.lastOp = "pause（静音）"
		st.lastOpAt = time.Now()
	case "resume":
		st.lastOp = "resume"
		st.lastOpAt = time.Now()
	case "seek":
		off := int64(body.Sec * float64(curBPS()))
		if off < 0 {
			off = 0
		}
		if off > int64(len(curSrc().PCM)) {
			off = int64(len(curSrc().PCM))
		}
		st.pcmPos = off
		st.lastOp = fmt.Sprintf("seek %.1fs", body.Sec)
		st.lastOpAt = time.Now()
	case "next", "prev":
		if body.Op == "next" {
			st.cur = (st.cur + 1) % len(srcs)
		} else {
			st.cur = (st.cur - 1 + len(srcs)) % len(srcs)
		}
		st.pcmPos = 0
		st.lastOp = body.Op
		st.lastOpAt = time.Now()
	default:
		mu.Unlock()
		http.Error(w, "unknown op", http.StatusBadRequest)
		return
	}
	cur := curSrc()
	op := body.Op
	sec := body.Sec
	mu.Unlock()

	// 投送新 URL：pause → 静音源；否则当前歌从 pcmPos 起
	var pushErr error
	switch op {
	case "pause":
		pushErr = rePushURL("sil", cur.Title, silDur, true)
	default:
		pushErr = rePushURL("song", cur.Title, cur.DurSec, false)
	}
	resp := stateMap()
	if pushErr != nil {
		log.Printf("[ctl] 投送失败: %v", pushErr)
		writeJSON(w, map[string]any{"ok": false, "err": pushErr.Error(), "state": resp})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "op": op, "sec": sec, "state": resp})
}

// rePushURL 投送目标音箱
func rePushURL(_ string, title string, dur float64, isSil bool) error {
	// 暂停态：静音全量；否则当前歌从 pcmPos 起
	mu.Lock()
	startSec := float64(st.pcmPos) / float64(curBPS())
	isPaused := st.lastOp == "pause（静音）"
	mu.Unlock()

	u := fmt.Sprintf("http://%s%s/stream?did=%s&v=%d", lanIP(), *flagPort, *flagDid, time.Now().UnixNano())
	if isSil || isPaused {
		u += "&sil=1"
	} else if startSec > 0 {
		u += fmt.Sprintf("&start=%.1f", startSec)
	}
	d := dur
	if isSil || isPaused {
		d = silDur
	}
	body := map[string]any{"url": u, "did": *flagDid, "title": title, "duration": d}
	b, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, *flagHC+"/api/v1/speaker/play_url", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("hcserver 返回 %d", resp.StatusCode)
	}
	mu.Lock()
	st.pushes++
	st.lastPushTime = time.Now()
	if st.lastLat == 0 && !st.lastOpAt.IsZero() {
		l := time.Since(st.lastOpAt).Milliseconds()
		if l >= 0 && l < 600000 {
			st.lastLat = l
		}
	}
	mu.Unlock()
	log.Printf("[push] %s（title=%s dur=%.0fs start=%.1fs sil=%v）", u, title, d, startSec, isSil||isPaused)
	return nil
}

// reconnector：播放中无新操作/total silence 场景已由全量模式覆盖，这里只做：距上次投送过久仍未判定 → 自动重投当前歌（断流兜底）
func reconnector() {
	for {
		time.Sleep(5 * time.Second)
		mu.Lock()
		cur := curSrc()
		paused := st.lastOp == "pause（静音）"
		t := st.lastPushTime
		mu.Unlock()
		// 投送过 且 距上次投送 > 4min 且 非暂停 → 续投当前歌（播完保护/断流兜底）
		if !t.IsZero() && !paused && time.Since(t) > 4*time.Minute {
			log.Printf("[reconnect] 无新投送>4min → 重投当前歌续播")
			if err := rePushURL("reconnect", cur.Title, cur.DurSec, false); err != nil {
				log.Printf("[reconnect] 失败: %v", err)
			}
		}
	}
}

func stateMap() map[string]any {
	mu.Lock()
	s := curSrc()
	pos := float64(st.pcmPos) / float64(s.Bps)
	if s.Bps == 0 {
		pos = 0
	}
	res := map[string]any{
		"cur":       st.cur,
		"title":     s.Title,
		"dur":       s.DurSec,
		"pos":       math.Round(pos*10) / 10,
		"lastOp":    st.lastOp,
		"lastLatMs": st.lastLat,
		"pushes":    st.pushes,
		"srcs":      len(srcs),
		"paused":    st.lastOp == "pause（静音）",
		"speakerUrl": fmt.Sprintf("http://%s%s/stream", lanIP(), *flagPort),
	}
	mu.Unlock()
	return res
}

var lanIPOnce sync.Once
var lanIPVal string

func lanIP() string {
	lanIPOnce.Do(func() {
		if *flagLanIP != "" {
			lanIPVal = *flagLanIP
			return
		}
		conn, err := net.Dial("udp", "8.8.8.8:80")
		if err == nil {
			lanIPVal = conn.LocalAddr().(*net.UDPAddr).IP.String()
			conn.Close()
		}
		if lanIPVal == "" {
			lanIPVal = "127.0.0.1"
		}
	})
	return lanIPVal
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func handlePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(pageHTML))
}

func handleState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, stateMap())
}

func mimeOf(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".m4s", ".m4a", ".aac", ".mp4":
		return "audio/mp4"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	default:
		return "application/octet-stream"
	}
}

func loadFile(path string) (*src, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	mime := mimeOf(path)
	hdr := []byte(nil)
	pcm := data
	if strings.HasSuffix(strings.ToLower(path), ".wav") && len(data) > 44 {
		hdr, pcm = data[:44], data[44:]
	}
	// 码率：默认 128k，mp4 用 64k 近似（容器码率未知）
	bps := int64(128 * 1000 / 8)
	if strings.HasSuffix(strings.ToLower(path), ".m4s") || strings.HasSuffix(strings.ToLower(path), ".m4a") {
		bps = int64(64 * 1000 / 8)
	}
	return &src{Title: filepath.Base(path), Header: hdr, PCM: pcm, Mime: mime, Bps: bps}, nil
}

func main() {
	flag.Parse()

	// 静音源加载
	if d, err := os.ReadFile(*flagSil); err == nil && len(d) > 0 {
		silData = d
		silMime = mimeOf(*flagSil)
		// 估算时长（96k=120s 对 sil120）
		silDur = 120
		log.Printf("[init] 静音源 %s %dB", *flagSil, len(d))
	} else {
		log.Printf("[warn] 无静音源 %s（暂停将不可用）", *flagSil)
	}

	// 源加载：优先 -songs，其次 -audio，最后内置
	if *flagSongs != "" {
		for i, item := range strings.Split(*flagSongs, ",") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			parts := strings.Split(item, ":")
			path := parts[0]
			dur := 180.0
			if len(parts) > 1 {
				dur, _ = strconv.ParseFloat(parts[1], 64)
			}
			s, err := loadFile(path)
			if err != nil {
				log.Printf("[skip] %s: %v", path, err)
				continue
			}
			s.ID = i
			s.DurSec = dur
			mu.Lock()
			srcs = append(srcs, s)
			mu.Unlock()
			log.Printf("[init] 曲%d %s: hdr=%d pcm=%d mime=%s bps=%d ~%.0fs", i, s.Title, len(s.Header), len(s.PCM), s.Mime, s.Bps, dur)
		}
	} else if *flagAudio != "" {
		s, err := loadFile(*flagAudio)
		if err != nil {
			log.Fatalf("读音频文件失败: %v", err)
		}
		s.DurSec = 180
		mu.Lock()
		srcs = append(srcs, s)
		mu.Unlock()
		log.Printf("[init] 单曲 %s", s.Title)
	} else {
		h1, p1 := genWAV(440, 60)
		h2, p2 := genWAV(660, 45)
		mu.Lock()
		srcs = append(srcs,
			&src{ID: 0, Title: "歌曲 A（440Hz 正弦）", DurSec: 60, Header: h1, PCM: p1, Mime: "audio/wav", Bps: sampleRate * 2},
			&src{ID: 1, Title: "歌曲 B（660Hz 正弦）", DurSec: 45, Header: h2, PCM: p2, Mime: "audio/wav", Bps: sampleRate * 2},
		)
		mu.Unlock()
		log.Printf("[init] 内置正弦 2 首")
	}
	if len(srcs) == 0 {
		log.Fatal("无可用音频源")
	}

	http.HandleFunc("/stream", handleStream)
	http.HandleFunc("/api/control", handleControl)
	http.HandleFunc("/api/state", handleState)
	http.HandleFunc("/", handlePage)
	go reconnector()
	log.Printf("spkdemo mock → http://127.0.0.1%s（全量200 + 操作换URL投送）", *flagPort)
	log.Printf("音箱拉流地址: http://%s%s/stream", lanIP(), *flagPort)
	if err := http.ListenAndServe(*flagPort, nil); err != nil {
		log.Fatal(err)
	}
}