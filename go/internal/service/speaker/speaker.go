// Package speaker 音箱域：小米音箱推送/本地模拟进度/自动连播（xiaomusic 方法论，零小米云轮询）。
// 全链路：入口 internal/api/speaker.go · hx/speaker.go；前端 speaker_*.html ×4 + speaker.js；表：无
package speaker

import (
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"sync"
	"time"

	"homecast/internal/service/token"
	"homecast/internal/bilibili"
	"homecast/internal/speaker"
)

// SpeakerService 音箱服务（对齐 Python speaker_service.py + device_manager.py）
type SpeakerService struct {
	Auth   *speaker.SpeakerAuth
	client *bilibili.Client
	tokens *token.TokenStore
	mu     sync.Mutex
	devices  map[string]speaker.SpeakerDevice
	playing  map[string]string // did -> url（统一端点 URL）
	blocked  map[string]bool   // did -> 断源（暂停/停止）：其拉流请求被拒
	progress map[string]*SpeakerProgress // did -> 本地模拟进度（零小米云轮询）
	port     string
	// EndedHook 播完回调（did, nextBVID）——重构后核心自动连播，不再自己递归 Play
	EndedHook func(did, nextBVID string)
}

// SpeakerProgress 推送播放的本地进度（用户思路：进度 = 拉流字节/码率，不查音箱）。
// LoadedBytes = proxy 已发送字节（音箱实际拉了多少流）；Bitrate = 音频码率bps。
// 分片喂流下拉取≈播放 → 字节/码率 ≈ 真实播放位置。回退本地时钟（无字节/码率时）。
type SpeakerProgress struct {
	BVID     string    `json:"bvid"`
	Title    string    `json:"title,omitempty"`
	Duration float64   `json:"duration"`             // 秒（前端已知优先，否则 B 站 view duration）
	NextBVID string    `json:"next_bvid,omitempty"`  // 播完自动连播的下一首
	Quality  int       `json:"-"`                    // 本次播放音质（自动连播/恢复沿用，删硬编码 30280）
	Bitrate  int       `json:"-"`                    // 音频码率bps（DASH bandwidth）——字节/码率=进度
	LoadedBytes int64  `json:"-"`                    // proxy 已发送字节（音箱实际拉流）——用户思路
	Started  time.Time `json:"-"`                    // 本次播放开始（本地时钟基准，回退用）
	PausedAt time.Time `json:"-"`                    // 暂停开始；零值=未暂停
	Paused   float64   `json:"-"`                    // 累计暂停秒（恢复时补偿）
	timer    *time.Timer `json:"-"`
	stopped  bool      `json:"-"`
}

// Position 当前位置（秒）：优先拉流字节/码率（音箱实际消费，用户思路），
// 无字节/码率时回退本地时钟（暂停中固定，播放中随本地时钟走）。
func (p *SpeakerProgress) Position() float64 {
	if p.Bitrate > 0 && p.LoadedBytes > 0 {
		return float64(p.LoadedBytes) / float64(p.Bitrate/8) // 字节/字节每秒 = 秒
	}
	if !p.PausedAt.IsZero() {
		return p.PausedAt.Sub(p.Started).Seconds() - p.Paused
	}
	return time.Since(p.Started).Seconds() - p.Paused
}

// State 播放状态
func (p *SpeakerProgress) State() string {
	if p == nil {
		return "idle"
	}
	if !p.PausedAt.IsZero() {
		return "paused"
	}
	return "playing"
}

// NewSpeakerService 构造
func NewSpeakerService(auth *speaker.SpeakerAuth, client *bilibili.Client, tokens *token.TokenStore) *SpeakerService {
	port := os.Getenv("HC_PORT")
	if port == "" {
		port = "28976"
	}
	return &SpeakerService{
		Auth:    auth,
		client:  client,
		tokens:  tokens,
		devices: map[string]speaker.SpeakerDevice{},
		playing: map[string]string{},
		blocked: map[string]bool{},
		progress: map[string]*SpeakerProgress{},
		port:    port,
	}
}

// ListDevices 设备列表（无则尝试刷新）
func (s *SpeakerService) ListDevices() []speaker.SpeakerDevice {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.devices) == 0 {
		if devs, err := s.Auth.ListDevices(); err == nil {
			for _, d := range devs {
				s.devices[d.DID] = d
			}
		}
	}
	out := make([]speaker.SpeakerDevice, 0, len(s.devices))
	for _, d := range s.devices {
		out = append(out, d)
	}
	return out
}

// RefreshDevices 强制刷新
func (s *SpeakerService) RefreshDevices() ([]speaker.SpeakerDevice, error) {
	devs, err := s.Auth.ListDevices()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.devices = map[string]speaker.SpeakerDevice{}
	for _, d := range devs {
		s.devices[d.DID] = d
	}
	s.mu.Unlock()
	return devs, nil
}

func (s *SpeakerService) getDevice(did string) (speaker.SpeakerDevice, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d, ok := s.devices[did]; ok {
		return d, true
	}
	// 兜底：老版本前端/WebView 缓存页提交的是 device_id（UUID）而非 did（MiotDID），
	// map 按 did 建索引会 404。按 device_id 反查一层，避免旧客户端/缓存页直接失效。
	for _, d := range s.devices {
		if d.DeviceID == did {
			return d, true
		}
	}
	return speaker.SpeakerDevice{}, false
}

// GetDevice 导出（API 层用）
func (s *SpeakerService) GetDevice(did string) (speaker.SpeakerDevice, bool) {
	return s.getDevice(did)
}

// localIP 音箱可访问的局域网 IP。
//
// 为什么不能只用 UDP 探测：本机同时有 Mihomo/Tailscale VPN 时，路由优先走 VPN 接口，
// net.Dial("udp","8.8.8.8") 返回 198.18.x.x / 100.64.x.x。小米 API 照例返回 code:0
// “消息已代理到设备”，但音箱在 192.168 网段根本访问不到 → 推送“成功”却无声。
// 所以不做人工配置，直接在网卡里打分挑最像家用局域网的那个。
func localIP() string {
	if ip := lanIP(); ip != "" {
		return ip
	}
	// 纯 VPN/容器环境找不到局域网地址：宁可报 127.0.0.1 失败也不要给个 VPN IP
	// 假装成功——音箱无声时比直接报错难查得多。
	log.Printf("[speaker] 未找到局域网 IP，音箱投送将不可用（纯 VPN/容器环境？）")
	return "127.0.0.1"
}

// lanIP 扫描网卡打分，返回最高分的 IPv4。0 分一律排除。
func lanIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	best, bestScore := "", 0
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipnet.IP.To4()
			if ip == nil {
				continue
			}
			if sc := scoreAddr(ip, iface.Flags); sc > bestScore {
				best, bestScore = ip.String(), sc
			}
		}
	}
	return best
}

// scoreAddr 给候选地址打分，越高越可能是家用局域网网卡；0 表示彻底排除。
func scoreAddr(ip net.IP, flags net.Flags) int {
	if ip == nil || len(ip) == 0 {
		return 0
	}
	a, b := ip[0], ip[1]
	switch {
	// link-local 169.254/16
	case a==169 && b==254,
		// Tailscale 100.64.0.0/10（b 在 64..127）
		a==100 && b&0xC0==64,
		// Mihomo/CGNAT 198.18.0.0/15（b 在 18..19）
		a==198 && b&0xFE==18:
		return 0
	}
	var score int
	switch {
	// 家用路由器几乎都用 192.168，docker 默认也不用它
	case a==192 && b==168:
		score = 100
	case a==10:
		score = 40
	case a==172 && b>=16 && b<=31:
		// docker 默认从 172.17/16 递增，172.17-20 基本是容器网桥
		if b>=17 && b<=20 {
			score = 5
		} else {
			score = 40
		}
	default:
		return 0
	}
	// 真实网卡带 BROADCAST；POINTOPOINT 隧道（Mihomo）没有
	if flags&net.FlagBroadcast != 0 {
		score += 20
	}
	return score
}

// unifiedAudioURL 音箱可访问的统一音频端点 URL（与 web 播放器同一个）。
// 源（B站 CDN / 缓存 / 本地文件）全由该端点内部解决，音箱不区分来源、不解析 B站、不用 token。
// did 带上，供断源闸门识别（暂停/停止 → 拒绝拉流）。
func (s *SpeakerService) unifiedAudioURL(bvid string, quality int, did string) string {
	return fmt.Sprintf("http://%s:%s/api/v1/music/stream/%s?quality=%d&did=%s",
		localIP(), s.port, bvid, quality, url.QueryEscape(did))
}

// BlockedStream 断源闸门：该 did 暂停/停止中 → 后续拉流被拒（音箱播完缓冲自然停）。
// 实现 api.StreamGate。
func (s *SpeakerService) BlockedStream(did string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.blocked[did]
}

// PlayOpts 推送可选参数（进度/自动连播用）
type PlayOpts struct {
	Title    string  // 歌曲名（前端已知；进度面板用）
	Duration float64 // 秒（前端本地播放器已知最准；0 则回退 B 站 view duration）
	NextBVID string  // 播完自动连播的下一首
}

// Play 播放到音箱：B站 bvid → DASH 音频 → 代理 URL → 音箱播放。
// 成功后建立本地模拟进度 + 自动连播定时器（xiaomusic/MiAir 同款方法论）。
func (s *SpeakerService) Play(bvid, did string, quality int, opts PlayOpts) (map[string]any, error) {
	device, ok := s.getDevice(did)
	if !ok {
		return map[string]any{"code": 404, "message": "设备未找到，请先刷新设备列表"}, nil
	}
	// 统一端点（与 web 播放器同一个）：缓存优先 + 未命中拉B站 + 回写缓存，全在端点内部
	url := s.unifiedAudioURL(bvid, quality, did)
	s.mu.Lock()
	s.playing[did] = url
	delete(s.blocked, did) // 重新播放 → 解除断源
	s.mu.Unlock()

	// 硬件分文：需要 music API 的设备（LX05A/L07A 等）用 play_by_music_url（带 audio_id），
	// 否则 play_by_url——对齐 MiAir/xiaomusic NEED_USE_PLAY_MUSIC_API 分支
	ret, err := s.Auth.PlayURL(device.DeviceID, url, device.Hardware)
	if err != nil {
		return map[string]any{"code": 500, "message": "音箱播放失败: " + err.Error()}, err
	}
	// 播放指令下发成功 → 建立本地模拟进度 + 自动连播定时器
	s.startProgress(did, bvid, opts, quality)
	log.Printf("[speaker] Play bvid=%s did=%s url=%s", bvid, did, url)
	return map[string]any{"code": 0, "message": "success", "data": ret}, nil
}

// startProgress 建立本地模拟进度（Started=now + duration 定时器）。
// 自动连播：duration 后触发下一首（NextBVID 非空）；duration 优先前端（本地播放器已知），
// 否则回退 B 站 view duration（自动连播场景）。
func (s *SpeakerService) startProgress(did, bvid string, opts PlayOpts, quality int) {
	duration := opts.Duration
	if duration <= 0 && s.client != nil {
		if info, err := s.client.GetVideoInfo(bvid); err == nil && info.Duration > 0 {
			duration = float64(info.Duration)
		}
	}
	title := opts.Title
	if title == "" && s.client != nil {
		if info, err := s.client.GetVideoInfo(bvid); err == nil && info.Title != "" {
			title = info.Title
		}
	}

	s.mu.Lock()
	old := s.progress[did]
	if old != nil && old.timer != nil {
		old.timer.Stop()
	}
	p := &SpeakerProgress{
		BVID:     bvid,
		Title:    title,
		Duration: duration,
		NextBVID: opts.NextBVID,
		Quality:  quality,
		Started:  time.Now(),
	}
	if duration > 0 && opts.NextBVID != "" {
		p.timer = time.AfterFunc(time.Duration(duration*float64(time.Second)), func() {
			s.playNextAuto(did, opts.NextBVID, quality)
		})
	}
	s.progress[did] = p
	s.mu.Unlock()
}

// RecordLoaded 记录音箱已拉流字节（proxy 回调）→ 进度 = 拉流字节/码率（用户思路）
func (s *SpeakerService) RecordLoaded(did string, bytes int64) {
	s.mu.Lock()
	p := s.progress[did]
	if p != nil {
		p.LoadedBytes = bytes
	}
	s.mu.Unlock()
}

// playNextAuto 自动连播：上一首播完（本地定时器到点）通知 EndedHook（核心统一走位）
func (s *SpeakerService) playNextAuto(did, nextBVID string, quality int) {
	log.Printf("[speaker] 定时器到点（本地模拟播完）→ 通知核心自动连播 next=%s did=%s", nextBVID, did)
	s.mu.Lock()
	hook := s.EndedHook
	s.mu.Unlock()
	if hook != nil {
		hook(did, nextBVID)
	}
}

// progressMap 组装进度 map（Progress/AllProgress 共用，删重复）
func sprogressMap(p *SpeakerProgress) map[string]any {
	pos := p.Position()
	if pos < 0 {
		pos = 0
	}
	return map[string]any{
		"state":     p.State(),
		"bvid":      p.BVID,
		"title":     p.Title,
		"position":  pos,
		"duration":  p.Duration,
		"next_bvid": p.NextBVID,
	}
}

// Progress 本地模拟进度（前端轮询本端点，不打小米云）
func (s *SpeakerService) Progress(did string) map[string]any {
	s.mu.Lock()
	p := s.progress[did]
	s.mu.Unlock()
	if p == nil || p.stopped {
		return map[string]any{"state": "idle", "bvid": "", "position": 0, "duration": 0}
	}
	return sprogressMap(p)
}

// AllProgress 所有设备本地模拟进度（{did: progress}，SSE 单流推所有，不打小米云）
func (s *SpeakerService) AllProgress() map[string]map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]map[string]any{}
	for did, p := range s.progress {
		if p == nil || p.stopped {
			continue
		}
		out[did] = sprogressMap(p)
	}
	return out
}

// Control 控制：pause/stop/resume/volume
func (s *SpeakerService) Control(did, action string, volume int) (map[string]any, error) {
	device, ok := s.getDevice(did)
	if !ok {
		return map[string]any{"code": 404, "message": "设备未找到"}, nil
	}
	var (
		ret any
		err error
	)
	switch action {
	case "pause":
		// 音箱 API 无暂停能力 → 断源：拒绝后续拉流，音箱播完已缓冲的自然停。
		s.mu.Lock()
		s.blocked[did] = true
		s.mu.Unlock()
		s.pauseProgress(did)
		ret = map[string]any{"paused": true}
	case "stop":
		s.mu.Lock()
		delete(s.playing, did)
		s.blocked[did] = true // 断源：停止后不再允许拉流
		s.stopProgressLocked(did)
		s.mu.Unlock()
		ret, err = s.Auth.Stop(device.DeviceID)
	case "resume", "play":
		if u, ok := s.playing[did]; ok {
			// 解除断源 + 重新下发播放指令 → 音箱重新拉流
			s.mu.Lock()
			delete(s.blocked, did)
			s.mu.Unlock()
			ret, err = s.Auth.PlayURL(device.DeviceID, u, device.Hardware)
			if err == nil {
				s.resumeProgress(did)
			}
		} else {
			return map[string]any{"code": 400, "message": "无已保存的播放 URL"}, nil
		}
	case "volume":
		if volume < 0 {
			return map[string]any{"code": 400, "message": "volume required"}, nil
		}
		ret, err = s.Auth.SetVolume(device.DeviceID, volume)
	default:
		return map[string]any{"code": 400, "message": "未知操作: " + action}, nil
	}
	if err != nil {
		return map[string]any{"code": 500, "message": "操作失败: " + err.Error()}, err
	}
	return map[string]any{"code": 0, "message": "success", "data": ret}, nil
}

// pauseProgress 暂停：断源 + 记 PausedAt + 停自动连播 timer（否则定时器照走→自动跳下一首）
func (s *SpeakerService) pauseProgress(did string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.progress[did]
	if p == nil || !p.PausedAt.IsZero() {
		return
	}
	p.PausedAt = time.Now()
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}
}

// resumeProgress 恢复：累计暂停秒 + 重起剩余定时器（播完自动连播仍有效）
func (s *SpeakerService) resumeProgress(did string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.progress[did]
	if p == nil || p.PausedAt.IsZero() {
		return
	}
	p.Paused += time.Since(p.PausedAt).Seconds()
	p.PausedAt = time.Time{}
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}
	if p.Duration > 0 && p.NextBVID != "" {
		remaining := p.Duration - p.Position()
		if remaining > 0 {
			p.timer = time.AfterFunc(time.Duration(remaining*float64(time.Second)), func() {
				s.playNextAuto(did, p.NextBVID, p.Quality)
			})
		}
	}
}

// stopProgressLocked 停止：清进度 + 停定时器（调用方需持锁）
func (s *SpeakerService) stopProgressLocked(did string) {
	p := s.progress[did]
	if p == nil {
		return
	}
	if p.timer != nil {
		p.timer.Stop()
	}
	p.stopped = true
	delete(s.progress, did)
}

// GetStatus 播放状态
func (s *SpeakerService) GetStatus(did string) (map[string]any, error) {
	device, ok := s.getDevice(did)
	if !ok {
		return map[string]any{"code": 404, "message": "设备未找到"}, nil
	}
	status, err := s.Auth.GetStatus(device.DeviceID)
	if err != nil {
		return map[string]any{"code": 500, "message": err.Error()}, err
	}
	if status == nil {
		return map[string]any{"code": 500, "message": "获取状态失败"}, nil
	}
	return map[string]any{"code": 0, "message": "success", "data": status}, nil
}

// SetVolume 音量
func (s *SpeakerService) SetVolume(did string, volume int) (map[string]any, error) {
	device, ok := s.getDevice(did)
	if !ok {
		return map[string]any{"code": 404, "message": "设备未找到"}, nil
	}
	if _, err := s.Auth.SetVolume(device.DeviceID, volume); err != nil {
		return map[string]any{"code": 500, "message": err.Error()}, err
	}
	return map[string]any{"code": 0, "message": "success", "data": map[string]any{"volume": volume}}, nil
}

