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
	failCount map[string]int  // did -> 连续播放失败次数（对齐 xiaomusic _play_failed_cnt，>=5 熔断）
	port     string
	// eventsSubs /speaker/events SSE 订阅者（服务端唯一 1s tick 广播所有设备进度——
	// 取代过去每连接自起 tick goroutine：N 个页面=N 个 tick 同时 AllProgress，
	// 与 /state/stream 双轨推同源进度；统一后与 PlaybackState/Widget 同构的订阅模型）
	eventsSubs map[chan map[string]map[string]any]struct{}
	pendingStops map[string]map[string]any // did -> idle 通知（停止时暂存，下次 tick 广播给前端设备卡刷回静止态）
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
	StartPos float64   `json:"-"`                    // 锚定后应处进度（新播 0 / 续播暂停点）
	anchorPending bool `json:"-"`                    // 等待音箱首次拉流校准（命令→拉流有延迟，虚增进度待扣）
	timer    *time.Timer `json:"-"`
	stopped  bool      `json:"-"`
}

// Position 当前位置（秒）：优先拉流字节/码率（音箱实际消费，用户思路），
// 无字节/码率时回退本地时钟（暂停中固定，播放中随本地时钟走）。
func (p *SpeakerProgress) Position() float64 {
	// 锚定前（命令下发→音箱真正拉流的窗口）：凝固在目标基准（新播0/续播暂停点），
	// 不按命令时刻虚增——否则拉流到达锚定瞬间进度从虚增值回跳（一开始播放就跳一下）。
	if p.anchorPending {
		return p.StartPos
	}
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
	s := &SpeakerService{
		Auth:    auth,
		client:  client,
		tokens:  tokens,
		devices: map[string]speaker.SpeakerDevice{},
		playing: map[string]string{},
		blocked: map[string]bool{},
		progress: map[string]*SpeakerProgress{},
		failCount: map[string]int{},
		port:    port,
		eventsSubs: map[chan map[string]map[string]any]struct{}{},
	}
	// 服务端唯一广播者：/speaker/events 订阅模型（取代每连接自带 tick）
	go s.progressBroadcaster()
	return s
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
// v=时间戳：每次播放/恢复/自动连播都是新 URL → 逼音箱重新拉流（对齐 xiaomusic 每次重新
// play_by_url 的事实行为；不变量会命中音箱缓存，恢复/切歌后音箱继续播旧内容/无声）。
func (s *SpeakerService) unifiedAudioURL(bvid string, quality int, did string) string {
	v := time.Now().UnixNano()
	return fmt.Sprintf("http://%s:%s/api/v1/music/stream/%s?quality=%d&did=%s&v=%d",
		localIP(), s.port, bvid, quality, url.QueryEscape(did), v)
}

// withFreshV 给任意播放 URL 刷新 v=时间戳（对齐 xiaomusic 每次 play 重新下发 URL）：
// 恢复/重播同一字符串也必须换 v，否则命中音箱缓存 → 播旧内容/无声。
func withFreshV(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	q.Set("v", fmt.Sprintf("%d", time.Now().UnixNano()))
	u.RawQuery = q.Encode()
	return u.String()
}

// withStart 给拉流 URL 带 start=<秒>（续播从中间开始；sec<=0 不带）
func withStart(raw string, sec float64) string {
	if sec <= 0 {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	q.Set("start", fmt.Sprintf("%.0f", sec))
	u.RawQuery = q.Encode()
	return u.String()
}

// resumePositionSec 续播位置：当前 progress 的 Position()（暂停中已含 Paused 补偿）。
// 只在有播放记录时返回 >0 的位置；否则 0（从头）。
func (s *SpeakerService) resumePositionSec(did string) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.progress[did]
	if p == nil {
		return 0
	}
	pos := p.Position()
	if pos < 0 {
		return 0
	}
	return pos
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

// Play 播放到音箱：B站 bvid → DASH 音频 → 代理 URL → 音箱播放（对齐 xiaomusic play：
// 先强停旧播放，再下发新 URL；失败累计熔断）。成功后本地进度 + 自动连播定时器。
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

	// 播放前强停旧播放（对齐 xiaomusic group_force_stop_xiaoai：先 player_pause 再停）
	if err := s.forceStopBeforePlay(device.DeviceID); err != nil {
		log.Printf("[speaker] 播放前强停(尽力而为) did=%s: %v", did, err)
	}

	// 硬件分文：需要 music API 的设备用 play_by_music_url（带 audio_id），
	// 否则 play_by_url——对齐 MiAir/xiaomusic NEED_USE_PLAY_MUSIC_API 分支
	ret, err := s.Auth.PlayURL(device.DeviceID, url, device.Hardware)
	if err != nil {
		s.recordFail(did)
		return map[string]any{"code": 500, "message": "音箱播放失败: " + err.Error()}, err
	}
	s.resetFail(did)
	// 播放指令下发成功 → 建立本地模拟进度 + 自动连播定时器
	s.startProgress(did, bvid, opts, quality)
	log.Printf("[speaker] Play bvid=%s did=%s url=%s", bvid, did, url)
	return map[string]any{"code": 0, "message": "success", "data": ret}, nil
}

// PlayURL 直接投任意 URL 到音箱（mock/测试/外部流用，不拼 B站统一端点）。
// 对齐 MiAir play_url：先强停旧播放再投新 URL；失败累计熔断。
func (s *SpeakerService) PlayURL(url, did string, opts PlayOpts) (map[string]any, error) {
	device, ok := s.getDevice(did)
	if !ok {
		return map[string]any{"code": 404, "message": "设备未找到，请先刷新设备列表"}, nil
	}
	s.mu.Lock()
	s.playing[did] = url
	delete(s.blocked, did) // 重新播放 → 解除断源
	s.mu.Unlock()

	if err := s.forceStopBeforePlay(device.DeviceID); err != nil {
		log.Printf("[speaker] PlayURL 前强停(尽力而为) did=%s: %v", did, err)
	}

	ret, err := s.Auth.PlayURL(device.DeviceID, url, device.Hardware)
	if err != nil {
		s.recordFail(did)
		return map[string]any{"code": 500, "message": "音箱播放失败: " + err.Error()}, err
	}
	s.resetFail(did)
	s.startProgress(did, url, opts, 0)
	log.Printf("[speaker] PlayURL url=%s did=%s", url, did)
	return map[string]any{"code": 0, "message": "success", "data": ret}, nil
}

// forceStopBeforePlay 播放前强停旧播放（对齐 xiaomusic group_force_stop_xiaoai）：
// 先 player_pause 让音箱停止消费，再连发 player_stop 彻底清空播放状态（MiAir stop 双保险同款）。
func (s *SpeakerService) forceStopBeforePlay(deviceID string) error {
	if _, err := s.Auth.Pause(deviceID, ""); err != nil {
		return err
	}
	_, err := s.Auth.Stop(deviceID)
	return err
}

// recordFail 累计连续失败（对齐 xiaomusic _play_failed_cnt：>=5 熔断）
func (s *SpeakerService) recordFail(did string) {
	s.mu.Lock()
	s.failCount[did]++
	n := s.failCount[did]
	s.mu.Unlock()
	log.Printf("[speaker] 播放失败累计 %d/5 did=%s", n, did)
	if n >= 5 {
		// 熔断：清状态，后续播放请求直接拒绝（对齐 xiaomusic 熔断保护）
		s.mu.Lock()
		delete(s.playing, did)
		s.blocked[did] = true
		s.failCount[did] = 0
		s.stopProgressLocked(did)
		s.mu.Unlock()
		log.Printf("[speaker] 连续 5 次播放失败，熔断停止 did=%s（需重新播放以解除）", did)
	}
}

// resetFail 播放成功 → 清零失败计数
func (s *SpeakerService) resetFail(did string) {
	s.mu.Lock()
	s.failCount[did] = 0
	s.mu.Unlock()
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
		// 新播等待音箱首次拉流校准（命令→拉流有延迟，锚定后进度才准）
		anchorPending: true,
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

// AllProgress 所有设备进度快照（/speaker/events 服务端单 tick 广播用）
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

// Subscribe /speaker/events SSE 订阅（服务端唯一 tick 推所有设备进度；订阅模型统一）
func (s *SpeakerService) Subscribe() chan map[string]map[string]any {
	ch := make(chan map[string]map[string]any, 8)
	s.mu.Lock()
	s.eventsSubs[ch] = struct{}{}
	s.mu.Unlock()
	return ch
}

// Unsubscribe 注销 /speaker/events 订阅者
func (s *SpeakerService) Unsubscribe(ch chan map[string]map[string]any) {
	s.mu.Lock()
	delete(s.eventsSubs, ch)
	s.mu.Unlock()
}

// progressBroadcaster 服务端唯一 1s tick：向 /speaker/events 订阅者广播所有设备进度
//（取代过去每个 SSE 连接自带 tick goroutine → N 页面=N 个协程同时 AllProgress；
// 与 PlaybackState/Widget 同构的订阅模型，非阻塞丢弃）
func (s *SpeakerService) progressBroadcaster() {
	tick := time.NewTicker(1 * time.Second)
	defer tick.Stop()
	for range tick.C {
		all := s.AllProgress()
		// 合并停止通知：设备被停/切走后面板没有活跃条目，但必须让前端把设备卡
		// 刷回静止态（否则 UI 永远停在“播放中”撒谎）。idle 通知在一次广播后过期。
		s.mu.Lock()
		if len(s.pendingStops) > 0 {
			if all == nil {
				all = map[string]map[string]any{}
			}
			for did, idle := range s.pendingStops {
				all[did] = idle
			}
			s.pendingStops = nil
		}
		s.mu.Unlock()
		if len(all) == 0 {
			continue
		}
		s.mu.Lock()
		for ch := range s.eventsSubs {
			select {
			case ch <- all:
			default: // 通道满则丢（前端未及时消费）
			}
		}
		s.mu.Unlock()
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
		// 对齐 MiAir/xiaomusic：pause 调 API（player_pause 真暂停）→ 音箱立即停声。
		// 断源 keep 为兜底：暂停后音箱若还拉旧流则拒绝（MiAir stop+pause 双保险同思路）。
		ret, err = s.Auth.Pause(device.DeviceID, device.Hardware)
		s.mu.Lock()
		s.blocked[did] = true
		s.mu.Unlock()
		s.pauseProgress(did)
		if err != nil {
			return map[string]any{"code": 500, "message": "操作失败: " + err.Error()}, err
		}
		ret = map[string]any{"paused": true}
	case "stop":
		s.mu.Lock()
		delete(s.playing, did)
		s.blocked[did] = true // 断源：停止后不再允许拉流
		s.stopProgressLocked(did)
		s.mu.Unlock()
		// 对齐 MiAir stop：player_stop + pause 双保险，彻底清空播放状态
		ret, err = s.Auth.Stop(device.DeviceID)
		if err == nil {
			_, err = s.Auth.Pause(device.DeviceID, device.Hardware)
		}
	case "resume", "play":
		if u, ok := s.playing[did]; ok {
			// 解除断源 + 重新下发播放指令。必须换新 v 强制音箱重拉
			// （对齐 xiaomusic 每次 play 重新下发 URL；同 URL 会命中音箱缓存 → 无声/播旧）
			fresh := withFreshV(u)
			// 续播：把暂停位置带上（fMP4 容器级 seek，音箱从中间开始）——暂停时
			// pause 记录位置（Paused 补偿），resume 转成 start=<秒> 让流端点切流。
			// 无暂停记录（如直接 play）不带 start，从头播。
			resumePos := s.resumePositionSec(did)
			fresh = withStart(fresh, resumePos)
			s.mu.Lock()
			s.playing[did] = fresh
			delete(s.blocked, did)
			s.mu.Unlock()
			ret, err = s.Auth.PlayURL(device.DeviceID, fresh, device.Hardware)
			if err != nil {
				s.recordFail(did)
			} else {
				s.resetFail(did)
				s.resumeProgress(did, resumePos)
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

// resumeProgress 恢复：累计暂停秒 + 重起剩余定时器（播完自动连播仍有效）。
// startPos = 续播位置（暂停点）：下一次音箱拉流（AnchorStream）到达时作为锚定目标。
func (s *SpeakerService) resumeProgress(did string, startPos float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.progress[did]
	if p == nil || p.PausedAt.IsZero() {
		return
	}
	p.Paused += time.Since(p.PausedAt).Seconds()
	p.PausedAt = time.Time{}
	// 续播后音箱会重新拉流（新 v 强制重拉）→ StartPos=暂停点，等待校准
	p.StartPos = startPos
	p.anchorPending = true
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

// AnchorStream 音箱首次拉流到达时校准进度基准（StreamGate 接口回调）。
// 原理：命令下发（resume/play）到音箱真正拉流存在延迟（网络+音箱解析），
// 若进度从命令时刻起算会虚增这段延迟——锚定后进度从拉流时刻起算：
//   Position(t) = 锚定时（将 Started 平移）使 Position(拉流时刻)=StartPos
//   （新播 StartPos=0；续播 StartPos=暂停点，两种都对上）。
// 拉流可能多次（断点重试），只锚定第一次：anchorPending 置 false。
func (s *SpeakerService) AnchorStream(did string, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.progress[did]
	if p == nil || !p.anchorPending {
		return
	}
	p.anchorPending = false
	// 目标：Position(at) == StartPos
	//   Position(t) = t.Sub(Started) - Paused   （暂停/恢复时 Paused 已累计）
	//   即 at.Sub(Started) - Paused == StartPos → Started = at - (StartPos + Paused)
	p.Started = at.Add(-time.Duration((p.StartPos + p.Paused) * float64(time.Second)))
	// 结束点也校准：自动连播定时器之前从命令时刻起算（会提前一个命令→拉流延迟），
	// 锚定后按真实拉流时刻 + 剩余时长重建，结束点对齐真实开播。
	if p.Duration > 0 && p.NextBVID != "" {
		if p.timer != nil {
			p.timer.Stop()
			p.timer = nil
		}
		remaining := p.Duration - p.StartPos
		if remaining > 0 {
			did, quality := did, p.Quality
			p.timer = time.AfterFunc(time.Duration(remaining*float64(time.Second)), func() {
				s.playNextAuto(did, p.NextBVID, quality)
			})
		}
	}
	log.Printf("[speaker] 拉流校准锚定 did=%s 进度基准=%.2fs (Started=%s 含补偿%.2fs)", did, p.StartPos, p.Started.Format("15:04:05"), p.Paused)
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
	// 先广播一条 idle 通知（前端设备卡刷回静止态），再移除条目。
	// 旧实现直接 delete：前端收不到任何事件，设备卡永远停在“播放中”（状态撒谎）。
	if s.pendingStops == nil {
		s.pendingStops = map[string]map[string]any{}
	}
	s.pendingStops[did] = map[string]any{"state": "idle", "bvid": "", "title": "", "position": 0, "duration": 0}
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

