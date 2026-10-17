// Package speaker 音箱域：小米音箱推送/本地模拟进度/自动连播（xiaomusic 方法论，零小米云轮询）。
// 全链路：入口 internal/api/speaker.go · hx/speaker.go；前端 speaker_*.html ×4 + speaker.js；表：无
package speaker

import (
	"fmt"
	"log"
	"net"
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
	playing  map[string]string // did -> url
	progress map[string]*SpeakerProgress // did -> 本地模拟进度（零小米云轮询）
	port     string
}

// SpeakerProgress 推送播放的本地模拟进度（xiaomusic/MiAir 同款方法论：
// 进度 = 本地时钟，自动连播 = 本地定时器，不打小米云）。
type SpeakerProgress struct {
	BVID     string    `json:"bvid"`
	Title    string    `json:"title,omitempty"`
	Duration float64   `json:"duration"`      // 秒（前端已知优先，否则 B 站 view duration）
	NextBVID string    `json:"next_bvid,omitempty"` // 播完自动连播的下一首
	Started  time.Time `json:"-"`             // 本次播放开始（本地时钟基准）
	PausedAt time.Time `json:"-"`             // 暂停开始；零值=未暂停
	Paused   float64   `json:"-"`             // 累计暂停秒（恢复时补偿）
	timer    *time.Timer `json:"-"`
	stopped  bool      `json:"-"`
}

// Position 本地模拟当前位置（秒）：暂停中固定，播放中随本地时钟走
func (p *SpeakerProgress) Position() float64 {
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

// proxyAudioURL 音箱可访问的代理 URL（局域网 IP + token）
func (s *SpeakerService) proxyAudioURL(bvid string, quality int, device speaker.SpeakerDevice) (string, string, error) {
	info, err := s.client.GetVideoInfo(bvid)
	if err != nil {
		return "", "", err
	}
	stream, err := s.client.GetBestAudioURL(bvid, info.CID, quality)
	if err != nil {
		return "", "", err
	}
	token := s.tokens.Store(stream.URL, map[string]string{
		"content_type": stream.MimeType,
		"original_url": stream.URL,
		"referer":      "https://www.bilibili.com/",
		"hardware":     device.Hardware,
		"mime_type":    stream.MimeType,
		"bvid":         bvid,
	})
	proxyURL := fmt.Sprintf("http://%s:%s/api/v1/proxy/audio/%s", localIP(), s.port, token)
	return proxyURL, stream.MimeType, nil
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
	proxyURL, _, err := s.proxyAudioURL(bvid, quality, device)
	if err != nil {
		return map[string]any{"code": 500, "message": "获取音频流失败: " + err.Error()}, err
	}
	s.mu.Lock()
	s.playing[did] = proxyURL
	s.mu.Unlock()

	// 部分硬件走 play_by_music_url（miservice-go 统一 PlayerPlayUrl）
	ret, err := s.Auth.PlayURL(device.DeviceID, proxyURL)
	if err != nil {
		return map[string]any{"code": 500, "message": "音箱播放失败: " + err.Error()}, err
	}
	// 播放指令下发成功 → 建立本地进度 + 自动连播定时器
	s.startProgress(did, bvid, opts, quality)
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

// playNextAuto 自动连播：上一首播完（本地定时器到点）推下一首
func (s *SpeakerService) playNextAuto(did, nextBVID string, quality int) {
	log.Printf("[speaker] 定时器到点（本地模拟播完）→ 自动连播下一首 %s did=%s", nextBVID, did)
	s.Play(nextBVID, did, quality, PlayOpts{}) // 递归建立新进度；title/duration 回退 B 站 view
}

// Progress 本地模拟进度（前端轮询本端点，不打小米云）
func (s *SpeakerService) Progress(did string) map[string]any {
	s.mu.Lock()
	p := s.progress[did]
	s.mu.Unlock()
	if p == nil || p.stopped {
		return map[string]any{"state": "idle", "bvid": "", "position": 0, "duration": 0}
	}
	pos := p.Position()
	if pos < 0 {
		pos = 0
	}
	return map[string]any{
		"state":    p.State(),
		"bvid":     p.BVID,
		"title":    p.Title,
		"position": pos,
		"duration": p.Duration,
		"next_bvid": p.NextBVID,
	}
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
		ret, err = s.Auth.Pause(device.DeviceID)
		if err == nil {
			s.pauseProgress(did)
		}
	case "stop":
		s.mu.Lock()
		delete(s.playing, did)
		s.stopProgressLocked(did)
		s.mu.Unlock()
		ret, err = s.Auth.Stop(device.DeviceID)
	case "resume", "play":
		if u, ok := s.playing[did]; ok {
			ret, err = s.Auth.PlayURL(device.DeviceID, u)
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

// pauseProgress 暂停：记 PausedAt（恢复时补偿）
func (s *SpeakerService) pauseProgress(did string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.progress[did]
	if p == nil || !p.PausedAt.IsZero() {
		return
	}
	p.PausedAt = time.Now()
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
				s.playNextAuto(did, p.NextBVID, 30280)
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

