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
	d, ok := s.devices[did]
	return d, ok
}

// GetDevice 导出（API 层用）
func (s *SpeakerService) GetDevice(did string) (speaker.SpeakerDevice, bool) {
	return s.getDevice(did)
}

// localIP 局域网 IP（对齐 Python UDP 探测）
func localIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
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