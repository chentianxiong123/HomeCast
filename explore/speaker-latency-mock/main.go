// sptest2 独立 mock + web：音箱播放，监测"音箱真正开始拉流"的时刻。
// 音频 = 程序生成的连续正弦音(WAV PCM)，第一字节开始就有声音（非静音），
// 音箱一拉流就能听到。web 页操控"播放/停止"，实时展示命令时刻→拉流时刻→延迟。
package main

import (
	"encoding/json"
	"bytes"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	miservice "github.com/lsongdev/miservice-go/miservice"
)

const (
	localSong = "/tmp/sptest2/songs/song.m4s"
	listenAddr = "0.0.0.0:28777" // mock 服务端口（web + 音频流）
	speakerIP  = "192.168.31.204"
)

type runState struct {
	mu       sync.Mutex
	Playing  bool      `json:"playing"`
	CmdTs    time.Time `json:"-"` // 命令真正发出
	FirstHit time.Time `json:"-"` // 音箱首次拉我们的流
	Hits     int       `json:"hits"`
	LastName string    `json:"lastName"`
	DelayMS  int64     `json:"-"`
	BufBytes int64     `json:"bufBytes"`
	AudioURL string    `json:"audioUrl"`
	Hz       float64   `json:"hz"`
	XcodeMode    bool      `json:"xcodeMode"` // 当前是否走实时转码
	XcodeFirstMs int64     `json:"xcodeFirstMs"` // 命令→ffmpeg首个mp3字节
	XcodeDone    bool      `json:"xcodeDone"`
	XcodeErr     string    `json:"xcodeErr"`
}

var st = &runState{}

// 独立 B 站 playurl 客户端：游客 buvid3 取音频流（单测过可拿真歌）
func bilibiliAudioURL(bvid string) (string, string, error) {
	buvid := genBuvid()
	// 1) 取 cid
	viewURL := "https://api.bilibili.com/x/web-interface/view?bvid=" + bvid
	req, _ := http.NewRequest("GET", viewURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Cookie", "buvid3="+buvid)
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("view: %v", err)
	}
	var jv struct {
		Code int `json:"code"`
		Data struct {
			Cid   int `json:"cid"`
			Pages []struct {
				Cid int `json:"cid"`
			} `json:"pages"`
		} `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&jv); err != nil {
		r.Body.Close()
		return "", "", err
	}
	r.Body.Close()
	cid := jv.Data.Cid
	if cid == 0 && len(jv.Data.Pages) > 0 {
		cid = jv.Data.Pages[0].Cid
	}
	if jv.Code != 0 || cid == 0 {
		return "", "", fmt.Errorf("view code=%d cid=%d", jv.Code, cid)
	}
	// 2) playurl
	pu := fmt.Sprintf("https://api.bilibili.com/x/player/playurl?bvid=%s&cid=%d&qn=%d&fnval=16&fourk=1", bvid, cid, 30280)
	req2, _ := http.NewRequest("GET", pu, nil)
	req2.Header.Set("User-Agent", "Mozilla/5.0")
	req2.Header.Set("Cookie", "buvid3="+buvid)
	r2, err := http.DefaultClient.Do(req2)
	if err != nil {
		return "", "", err
	}
	defer r2.Body.Close()
	var jp struct {
		Code int `json:"code"`
		Data struct {
			Dash struct {
				Audio []struct {
					ID      int    `json:"id"`
					BaseURL string `json:"baseUrl"`
				} `json:"audio"`
			} `json:"dash"`
		} `json:"data"`
	}
	if err := json.NewDecoder(r2.Body).Decode(&jp); err != nil {
		return "", "", err
	}
	for _, a := range jp.Data.Dash.Audio {
		if a.ID == 30280 || a.ID == 30232 || a.ID == 30216 {
			if a.BaseURL != "" {
				return a.BaseURL, buvid, nil
			}
		}
	}
	if len(jp.Data.Dash.Audio) > 0 {
		return jp.Data.Dash.Audio[0].BaseURL, buvid, nil
	}
	return "", "", fmt.Errorf("no audio in dash")
}

// genBuvid 生成合法 UUID 格式的 buvid3（B站校验格式，字面占位符会 403）
func genBuvid() string {
	b := make([]byte, 16)
	n := time.Now().UnixNano()
	for i := range b {
		n = n*6364136223846793005 + 1442695040888963407
		b[i] = byte(n >> 33)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func rng() string {
	b := make([]byte, 13)
	for i := range b {
		b[i] = "0123456789abcdef"[time.Now().UnixNano()%16]
	}
	return string(b)
}

func genTone(dur float64, hz float64, sampleRate int) []byte {
	n := int(dur * float64(sampleRate))
	buf := make([]byte, n*2)
	amp := 0.3 * 32767
	phase := 0.0
	incr := 2 * math.Pi * hz / float64(sampleRate)
	fadeN := sampleRate / 100
	for i := 0; i < n; i++ {
		gain := 1.0
		if i < fadeN {
			gain = float64(i) / float64(fadeN)
		}
		v := int16(amp * gain * math.Sin(phase))
		buf[i*2] = byte(v)
		buf[i*2+1] = byte(v >> 8)
		phase += incr
	}
	return buf
}

func audioStreamHandler(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	st.mu.Lock()
	first := st.FirstHit.IsZero()
	if first {
		st.FirstHit = now
		if !st.CmdTs.IsZero() {
			st.DelayMS = now.Sub(st.CmdTs).Milliseconds()
		}
	}
	st.Hits++
	st.Playing = true
	st.AudioURL = r.URL.RequestURI()
	h := st.Hits
	last := st.LastName
	st.mu.Unlock()
	if first {
		log.Printf("[拉流 #%d] %s %s (命令后 delay=%dms)", h, now.Format("15:04:05.000"), r.URL.RequestURI(), st.DelayMS)
	} else {
		log.Printf("[拉流 #%d] %s %s (续)", h, now.Format("15:04:05.000"), last)
	}
	// 打印拉流请求的 Range 头（关键：音箱是否从头重拉/续传）
	log.Printf("[拉流 Range] %s Range=%q", now.Format("15:04:05.000"), r.Header.Get("Range"))

	w.Header().Set("Access-Control-Allow-Origin", "*")
	// 代理真 B 站：从 bvid 取音频转发给音箱
	bvid := strings.TrimPrefix(r.URL.Path, "/audio/")
	if i := indexByte(bvid, '.'); i >= 0 {
		bvid = bvid[:i]
	}
	if bvid == "" {
		w.WriteHeader(400)
		return
	}
	upURL, _, err := bilibiliAudioURL(bvid)
	if err != nil {
		log.Printf("[mock] 取 B 站音频失败 %s: %v", bvid, err)
		w.WriteHeader(502)
		return
	}
	upReq, _ := http.NewRequest("GET", upURL, nil)
	upReq.Header.Set("User-Agent", "Mozilla/5.0")
	upReq.Header.Set("Referer", "https://www.bilibili.com") // B站 CDN 防盗链
	if rg := r.Header.Get("Range"); rg != "" {
		upReq.Header.Set("Range", rg)
	}
	upResp, err := http.DefaultClient.Do(upReq)
	if err != nil {
		log.Printf("[mock] 上游取流失败: %v", err)
		w.WriteHeader(502)
		return
	}
	defer upResp.Body.Close()
	for _, k := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges"} {
		if v := upResp.Header.Get(k); v != "" {
			w.Header().Set(k, v)
		}
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "audio/mpeg")
	}
	w.WriteHeader(upResp.StatusCode)
	rdBuf := make([]byte, 8192)
	for {
		n, e := upResp.Body.Read(rdBuf)
		if n > 0 {
			if _, werr := w.Write(rdBuf[:n]); werr != nil {
				break
			}
			st.mu.Lock()
			st.BufBytes += int64(n)
			st.mu.Unlock()
		}
		if e != nil {
			break
		}
	}
}


func localStreamHandler(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	st.mu.Lock()
	first := st.FirstHit.IsZero()
	if first {
		st.FirstHit = now
		if !st.CmdTs.IsZero() {
			st.DelayMS = now.Sub(st.CmdTs).Milliseconds()
		}
	}
	st.Hits++
	st.Playing = true
	st.AudioURL = r.URL.RequestURI()
	h := st.Hits
	st.mu.Unlock()
	if first {
		log.Printf("[本地拉流 #%d] %s %s (命令后 delay=%dms)", h, now.Format("15:04:05.000"), r.URL.RequestURI(), st.DelayMS)
	} else {
		log.Printf("[本地拉流 #%d] %s (续)", h, now.Format("15:04:05.000"))
	}
	log.Printf("[本地拉流 Range] %s Range=%q", now.Format("15:04:05.000"), r.Header.Get("Range"))

	name := strings.TrimPrefix(r.URL.Path, "/local/")
	if strings.Contains(name, "/") || name == "" {
		w.WriteHeader(400)
		return
	}
	fpath := "/tmp/sptest2/songs/" + name
	f, err := os.Open(fpath)
	if err != nil {
		w.WriteHeader(500)
		return
	}
	defer f.Close()
	fi, _ := f.Stat()
	fileSize := fi.Size()

	start, end := int64(0), fileSize-1
	fullBody := true
	if rg := r.Header.Get("Range"); rg != "" {
		var s, e int64
		if _, err := fmt.Sscanf(rg, "bytes=%d-%d", &s, &e); err == nil {
			start, end = s, e
			fullBody = false
		} else if _, err := fmt.Sscanf(rg, "bytes=%d-", &s); err == nil {
			start = s
			fullBody = false
		}
	}
	if start < 0 || start >= fileSize {
		w.WriteHeader(416)
		return
	}
	if end >= fileSize {
		end = fileSize - 1
	}
	length := end - start + 1

	if !fullBody {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, fileSize))
		w.WriteHeader(206)
	} else {
		w.WriteHeader(200)
	}
	w.Header().Set("Content-Length", fmt.Sprintf("%d", length))
	ct := "audio/mpeg"
	if strings.HasSuffix(name, ".m4s") || strings.HasSuffix(name, ".mp4") {
		ct = "audio/mp4"
	} else if strings.HasSuffix(name, ".flac") {
		ct = "audio/flac"
	} else if strings.HasSuffix(name, ".wav") {
		ct = "audio/wav"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Accept-Ranges", "bytes")
	f.Seek(start, 0)
	remain := length
	rdBuf := make([]byte, 8192)
	for remain > 0 {
		rdLen := len(rdBuf)
		if int64(rdLen) > remain {
			rdLen = int(remain)
		}
		n, e := io.ReadFull(f, rdBuf[:rdLen])
		if n > 0 {
			if _, werr := w.Write(rdBuf[:n]); werr != nil {
				return
			}
			st.mu.Lock()
			st.BufBytes += int64(n)
			st.mu.Unlock()
			remain -= int64(n)
		}
		if e != nil {
			break
		}
	}
}


// getM4s 确保该 bvid 的 m4s 已缓存到本地，返回本地路径（Go 下载，带 referer，绕开 ffmpeg 403）
func getM4s(bvid string) (string, error) {
	m4sPath := "/tmp/sptest2/songs/" + bvid + ".m4s"
	if fi, e := os.Stat(m4sPath); e == nil && fi.Size() > 0 {
		return m4sPath, nil
	}
	upURL, _, err := bilibiliAudioURL(bvid)
	if err != nil {
		return "", err
	}
	req, _ := http.NewRequest("GET", upURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Referer", "https://www.bilibili.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		return "", fmt.Errorf("上游 %d", resp.StatusCode)
	}
	f, err := os.Create(m4sPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", err
	}
	return m4sPath, nil
}

// xcodeStreamHandler 边转边喂 + 缓存解耦：按 bvid 下载 m4s → ffmpeg 转 mp3 →
// mp3 输出 tee 两路（实时推音箱出声 + 落缓存）。音箱拉到第一段即出声，拉够停拉不影响转码落盘。
func xcodeStreamHandler(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	st.mu.Lock()
	first := st.FirstHit.IsZero()
	if first {
		st.FirstHit = now
		if !st.CmdTs.IsZero() {
			st.DelayMS = now.Sub(st.CmdTs).Milliseconds()
		}
	}
	st.Hits++
	st.Playing = true
	st.XcodeMode = true
	st.AudioURL = r.URL.RequestURI()
	h := st.Hits
	st.mu.Unlock()
	if first {
		log.Printf("[转码拉流 #%d] %s %s (命令后 delay=%dms)", h, now.Format("15:04:05.000"), r.URL.RequestURI(), st.DelayMS)
	} else {
		log.Printf("[转码拉流 #%d] %s (续)", h, now.Format("15:04:05.000"))
	}

	bvid := strings.TrimPrefix(r.URL.Path, "/xcode/")
	if i := indexByte(bvid, '.'); i >= 0 {
		bvid = bvid[:i]
	}
	if bvid == "" {
		w.WriteHeader(400)
		return
	}
	st.mu.Lock()
	st.XcodeFirstMs = 0
	st.mu.Unlock()

	dst := "/tmp/sptest2/songs/" + bvid + ".mp3"
	// 已有完整缓存 mp3 → 直接喂（第二次播秒发）
	if fi, e := os.Stat(dst); e == nil && fi.Size() > 0 {
		log.Printf("[xcode] 命中缓存mp3(%d字节) 直接喂", fi.Size())
		http.ServeFile(w, r, dst)
		return
	}

	// 确保源 m4s 在本地（Go 下载绕开 ffmpeg 403）
	src, err := getM4s(bvid)
	if err != nil {
		log.Printf("[xcode] 取m4s失败 %s: %v", bvid, err)
		w.WriteHeader(502)
		return
	}

	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Accept-Ranges", "none")
	w.WriteHeader(200)
	flusher, _ := w.(http.Flusher)

	pr, pw := io.Pipe()
	cmd := exec.Command("ffmpeg", "-y", "-hide_banner", "-loglevel", "error",
		"-i", src, "-vn", "-c:a", "libmp3lame", "-b:a", "128k", "-f", "mp3", "pipe:1")
	cmd.Stdout = pw
	var eb bytes.Buffer
	cmd.Stderr = &eb
	go func() {
		defer pw.Close()
		if err := cmd.Run(); err != nil {
			log.Printf("[xcode] ffmpeg 退出 err=%v stderr=%s", err, eb.String())
		}
	}()

	cacheF, err := os.Create(dst + ".tmp")
	if err != nil {
		log.Printf("[xcode] 建缓存失败: %v", err)
		w.WriteHeader(500)
		return
	}
	defer cacheF.Close()
	outBuf := make([]byte, 8192)
	written := int64(0)
	firstByteAt := time.Time{}
	for {
		n, e := pr.Read(outBuf)
		if n > 0 {
			if firstByteAt.IsZero() {
				firstByteAt = time.Now()
				st.mu.Lock()
				if st.CmdTs.IsZero() {
					st.XcodeFirstMs = 0
				} else {
					st.XcodeFirstMs = firstByteAt.Sub(st.CmdTs).Milliseconds()
				}
				st.mu.Unlock()
				log.Printf("[xcode] 首个mp3字节 %s (命令后 %dms) → 音箱即出声", firstByteAt.Format("15:04:05.000"), st.XcodeFirstMs)
			}
			cacheF.Write(outBuf[:n])
			if _, werr := w.Write(outBuf[:n]); werr != nil {
				log.Printf("[xcode] 音箱停止拉流(已喂%d字节)，转码后台继续落缓存", written)
				io.Copy(io.Discard, pr)
				cacheF.Close()
				os.Rename(dst+".tmp", dst)
				st.mu.Lock()
				st.XcodeDone = true
				st.mu.Unlock()
				log.Printf("[xcode] 缓存已落地(音箱侧断)")
				return
			}
			written += int64(n)
			st.mu.Lock()
			st.BufBytes += int64(n)
			st.mu.Unlock()
			if flusher != nil {
				flusher.Flush()
			}
		}
		if e != nil {
			break
		}
	}
	cacheF.Close()
	os.Rename(dst+".tmp", dst)
	st.mu.Lock()
	st.XcodeDone = true
	st.mu.Unlock()
	log.Printf("[xcode] 边转边喂完成 共喂音箱%d字节，缓存落地", written)
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func wavHeader(sampleRate, seconds int) []byte {
	dataSize := sampleRate * 2 * seconds
	buf := make([]byte, 44)
	copy(buf[0:4], "RIFF")
	le32(buf[4:8], uint32(36+dataSize))
	copy(buf[8:12], "WAVE")
	copy(buf[12:16], "fmt ")
	le32(buf[16:20], 16)
	le16(buf[20:22], 1)
	le16(buf[22:24], 1)
	le32(buf[24:28], uint32(sampleRate))
	le32(buf[28:32], uint32(sampleRate*2))
	le16(buf[32:34], 2)
	le16(buf[34:36], 16)
	copy(buf[36:40], "data")
	le32(buf[40:44], uint32(dataSize))
	return buf
}
func le16(b []byte, v uint16) { b[0] = byte(v); b[1] = byte(v >> 8) }
func le32(b []byte, v uint32) { b[0] = byte(v); b[1] = byte(v >> 8); b[2] = byte(v >> 16); b[3] = byte(v >> 24) }

func loadToken() *miservice.Tokens {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".config", "homecast", "speaker.json")
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("读 token %s: %v", path, err)
	}
	t := miservice.NewTokens()
	if err := json.Unmarshal(data, t); err != nil {
		log.Fatalf("解析 token: %v", err)
	}
	return t
}

func ts(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("15:04:05.000")
}

func main() {
	var mu sync.Mutex
	var client *miservice.Client
	var devices []miservice.DeviceData

	ensureClient := func() {
		if client == nil {
			client = miservice.NewClient("", "")
			client.Token = loadToken()
			if err := client.Login("micoapi"); err != nil {
				log.Printf("登录失败: %v", err)
				return
			}
			devices, _ = client.ListMinaDevices(0)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(webPage()))
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/audio/", audioStreamHandler)
	mux.HandleFunc("/local/", localStreamHandler)
	mux.HandleFunc("/xcode/", xcodeStreamHandler)

	mux.HandleFunc("/api/devices", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		ensureClient()
		w.Header().Set("Content-Type", "application/json")
		out := []map[string]any{}
		for i := range devices {
			d := &devices[i]
			if d.Presence == "online" {
				didNo := ""
				if d.MiotDID != "" {
					didNo = d.MiotDID
				}
				out = append(out, map[string]any{"device_id": d.DeviceID, "name": d.Name, "hardware": d.Hardware, "did": didNo, "online": true})
			}
		}
		json.NewEncoder(w).Encode(out)
	})

	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		st.mu.Lock()
		defer st.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"playing": st.Playing, "cmdTs": ts(st.CmdTs), "firstHit": ts(st.FirstHit),
			"delayMs": st.DelayMS, "hits": st.Hits, "bufBytes": st.BufBytes,
			"audioUrl": st.AudioURL, "hz": st.Hz, "lastName": st.LastName,
		})
	})

	mux.HandleFunc("/api/play", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			DeviceID string `json:"device_id"`
			Hz       float64 `json:"hz"`
			BVID     string  `json:"bvid"`
			Path     string  `json:"path"`
		Xcode    bool    `json:"xcode"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if body.DeviceID == "" || (body.BVID == "" && body.Path == "") {
			w.Write([]byte(`{"error":"device_id / (bvid|path) 必填"}`))
			return
		}
		mu.Lock()
		defer mu.Unlock()
		ensureClient()
		if client == nil {
			w.Write([]byte(`{"error":"客户端未就绪"}`))
			return
		}
		// 记录命令发出时刻
		now := time.Now()
		st.mu.Lock()
		st.CmdTs = now
		st.FirstHit = time.Time{}
		st.Hits = 0
		st.BufBytes = 0
		st.DelayMS = -1
		st.Playing = false
		st.Hz = body.Hz
		st.mu.Unlock()
		stream := "/audio/" + body.BVID
		if body.Path != "" {
			stream = body.Path
		}
		if body.Xcode {
			stream = "/xcode/" + body.BVID
			st.mu.Lock()
			st.XcodeMode = true
			st.XcodeDone = false
			st.XcodeFirstMs = 0
			st.mu.Unlock()
		}
		url := fmt.Sprintf("http://%s:28777%s?v=%d", speakerIP, stream, now.UnixMilli())
		_, err := client.PlayerPlayUrl(body.DeviceID, url)
		if err != nil {
			st.mu.Lock()
			st.LastName = "下发失败: " + err.Error()
			st.mu.Unlock()
			w.Write([]byte(`{"error":"` + err.Error() + `"}`))
			return
		}
		st.mu.Lock()
		st.LastName = "已下发"
		st.mu.Unlock()
		log.Printf("[web] 命令下发 %s -> %s", now.Format("15:04:05.000"), body.DeviceID)
		w.Write([]byte(`{"ok":true}`))
	})

	mux.HandleFunc("/api/stop", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			DeviceID string `json:"device_id"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		defer mu.Unlock()
		ensureClient()
		if client == nil {
			w.Write([]byte(`{"error":"客户端未就绪"}`))
			return
		}
		if body.DeviceID == "" {
			w.Write([]byte(`{"error":"device_id 必填"}`))
			return
		}
		// 只停用户选中的这一台，绝不动另一台
		client.PlayerPause(body.DeviceID)
		client.PlayerStop(body.DeviceID)
		st.mu.Lock()
		st.Playing = false
		st.mu.Unlock()
		log.Printf("[stop] 停止设备 %s", body.DeviceID)
		w.Write([]byte(`{"ok":true,"device_id":"` + body.DeviceID + `"}`))
	})

	log.Printf("sp-mock web + 音频流 http://%s", listenAddr)
	http.ListenAndServe(listenAddr, mux)
}

func webPage() string {
	return `<!DOCTYPE html><html lang=zh><head><meta charset=utf-8>
<meta name=viewport content="width=device-width,initial-scale=1"><title>音箱拉流延迟测试台</title>
<style>body{font-family:system-ui;background:#0f1115;color:#e5e7eb;padding:24px;max-width:680px;margin:0 auto}
h1{font-size:20px}.card{background:#1a1d24;border:1px solid #2a2e37;border-radius:10px;padding:16px;margin:14px 0}
.row{display:flex;gap:10px;align-items:center;flex-wrap:wrap;padding:6px 0}
.btn{padding:12px 20px;border:none;border-radius:8px;font-size:15px;cursor:pointer;font-weight:600}
.play{background:#2563eb;color:#fff}.stop{background:#dc2626;color:#fff}
.big{font-size:26px;font-weight:700;color:#3b82f6;margin-left:auto}
label{display:block;margin:10px 0 4px;font-size:13px;color:#9ca3af}
select,input{background:#0f1115;color:#e5e7eb;border:1px solid #2a2e37;border-radius:6px;padding:8px;width:100%}
.mono{font-family:monospace;color:#9ca3af}pre{background:#0f1115;padding:10px;border-radius:6px;max-height:200px;overflow:auto;font-size:12px}
</style></head><body><h1>音箱拉流延迟测试台（mock，独立）</h1>
<div class=card>
<label>音箱设备</label><select id=dev></select>
<label>B站 BV 号</label><input id=bv value=BV1ciWb6yEWW>
<div class=row style=margin-top:14px>
<button class="btn play" onclick=play()>▶ 播放到音箱(B站)</button>
<button class="btn play" onclick=playLocal()>▶ 本地m4s</button>
<button class="btn play" onclick=playMp3()>▶ 本地mp3</button>
<button class="btn play" style=background:#059669 onclick=playXcode()>▶ 实时转码mp3推(B站m4s→mp3)</button>
<button class="btn stop" onclick=stop()>■ 停止</button>
</div></div>
<div class=card>
<div class=row><div>命令下发时刻</div><div class=mono id=cTs>-</div></div>
<div class=row><div>音箱首次拉流</div><div class=mono id=fTs>-</div></div>
<div class=row><div>动态延迟</div><div class=big id=del>-</div></div>
<div class=row><div>累计拉流</div><div class=mono id=hits>0 次</div></div>
<div class=row><div>已喂字节</div><div class=mono id=buf>0 B</div></div>
<div class=row><div>转码首字节(命令后)</div><div class=mono id=xms>-</div></div>
<div class=row><div>音频 URL</div><div class=mono id=url>-</div></div>
<div class=row><div>日志</div></div><pre id=log></pre>
</div>
<script>let devs=[];
async function ld(){let r=await fetch('/api/devices');devs=await r.json();let s=document.getElementById('dev');s.innerHTML='';devs.forEach(d=>{let o=document.createElement('option');o.value=d.device_id;o.textContent=d.name+' ('+d.hardware+')';s.appendChild(o)})}
async function play(){let d=document.getElementById('dev').value;let bv=document.getElementById('bv').value;
let r=await fetch('/api/play',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({device_id:d,bvid:bv})});let j=await r.json();logl('play: '+JSON.stringify(j))}
async function playXcode(){let d=document.getElementById('dev').value;let bv=document.getElementById('bv').value;
let r=await fetch('/api/play',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({device_id:d,xcode:true,bvid:bv})});let j=await r.json();logl('转码play: '+JSON.stringify(j))}
async function playMp3(){let d=document.getElementById('dev').value;
let r=await fetch('/api/play',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({device_id:d,path:'/local/song.mp3'})});let j=await r.json();logl('mp3play: '+JSON.stringify(j))}
async function playLocal(){let d=document.getElementById('dev').value;
let r=await fetch('/api/play',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({device_id:d,path:'/local/song.m4s'})});let j=await r.json();logl('本地play: '+JSON.stringify(j))}
async function stop(){let d=document.getElementById('dev').value;let r=await fetch('/api/stop',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({device_id:d})});logl('stop: '+JSON.stringify(await r.json()))}
async function poll(){let r=await fetch('/api/state');let s=await r.json();
cTs.textContent=s.cmdTs;fTs.textContent=s.firstHit;
del.textContent=(s.delayMs>=0)?s.delayMs+' ms':'-';
hits.textContent=s.hits+' 次';buf.textContent=fb(s.bufBytes);
url.textContent=s.audioUrl;
xms.textContent=s.xcodeFirstMs?s.xcodeFirstMs+" ms":"-";
}
function fb(n){if(n>1e6)return(n/1e6).toFixed(1)+' MB';if(n>1e3)return(n/1e3).toFixed(1)+' KB';return n+' B'}
function logl(s){let e=document.createElement('div');e.textContent=new Date().toTimeString().slice(0,12)+' '+s;document.getElementById('log').prepend(e)}
ld();setInterval(poll,250);
</script></body></html>`
}
