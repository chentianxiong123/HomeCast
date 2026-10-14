package service

import (
	"fmt"
	"net"
	"os"
	"strings"
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

// localIP 音箱可访问的局域网 IP（对齐 Python config.public_url，但加自动探测）。
//
// 为什么不能只用 UDP 探测：本机有 Mihomo/Tailscale 等 VPN 时，路由优先走 VPN 接口，
// net.Dial("udp","8.8.8.8") 会返回 198.18.x.x / 100.64.x.x。小米 API 返回 code:0
// “消息已代理到设备”，但音箱在 192.168 网段根本访问不到该 IP → 推送“成功”却无声。
//
// 优先级：HC_PUBLIC_IP 环境变量 > 网卡扫描 RFC1918 > UDP 探测 > 127.0.0.1。
// 用户明确知道局域网地址时，export HC_PUBLIC_IP=192.168.31.9 即可锁定。
func localIP() string {
	if v := strings.TrimSpace(os.Getenv("HC_PUBLIC_IP")); v != "" {
		return v
	}
	if ip := lanIP(); ip != "" {
		return ip
	}
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}

// lanIP 扫描网卡，返回第一个 RFC1918 IPv4（10 / 172.16-31 / 192.168），
// 跳过回环、link-local、以及 Tailscale(100.64/10) 与 Mihomo(198.18/15) VPN 段。
func lanIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
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
			if ip != nil && isLanIPv4(ip) {
				return ip.String()
			}
		}
	}
	return ""
}

// isLanIPv4 仅接受家用局域网地址，排除 VPN/容器网段。
func isLanIPv4(ip net.IP) bool {
	if ip == nil {
		return false
	}
	a, b := ip[0], ip[1]
	switch {
	case a == 10:
		return true
	case a == 172 && b >= 16 && b <= 31:
		return true
	case a == 192 && b == 168:
		return true
	}
	return false
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

var _ = strings.TrimSpace