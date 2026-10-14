package service

import (
	"fmt"
	"log"
	"net"
	"os"
	"sync"

	"homecast/internal/bilibili"
	"homecast/internal/speaker"
)

// SpeakerService 音箱服务（对齐 Python speaker_service.py + device_manager.py）
type SpeakerService struct {
	Auth   *speaker.SpeakerAuth
	client *bilibili.Client
	tokens *TokenStore
	mu     sync.Mutex
	devices map[string]speaker.SpeakerDevice
	playing map[string]string // did -> url
	port    string
}

// NewSpeakerService 构造
func NewSpeakerService(auth *speaker.SpeakerAuth, client *bilibili.Client, tokens *TokenStore) *SpeakerService {
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

// Play 播放到音箱：B站 bvid → DASH 音频 → 代理 URL → 音箱播放
func (s *SpeakerService) Play(bvid, did string, quality int) (map[string]any, error) {
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
	return map[string]any{"code": 0, "message": "success", "data": ret}, nil
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
	case "stop":
		s.mu.Lock()
		delete(s.playing, did)
		s.mu.Unlock()
		ret, err = s.Auth.Stop(device.DeviceID)
	case "resume", "play":
		if u, ok := s.playing[did]; ok {
			ret, err = s.Auth.PlayURL(device.DeviceID, u)
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

