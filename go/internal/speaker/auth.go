// Package speaker 小米小爱音箱（对齐 Python speaker/auth.py + device_manager.py）
//
// 登录方式：
//  1. 账号密码（miservice-go 原生）
//  2. 已有 token 恢复（~/.config/homecast/speaker.json，miservice.Tokens JSON）
//  3. 二维码登录（qrcode.go，走 account.xiaomi.com 长轮询）
package speaker

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/lsongdev/miservice-go/miservice"
)

// SpeakerDevice 音箱设备（对齐 Python SpeakerDevice + is_online）
type SpeakerDevice struct {
	DeviceID string `json:"device_id"`
	Hardware string `json:"hardware"`
	DID      string `json:"did"`
	Name     string `json:"name"`
	IsOnline bool   `json:"is_online"`
}

// 部分硬件需要用 play_music_api（对齐 Python NEED_USE_PLAY_MUSIC_API）
var needUsePlayMusicAPI = map[string]bool{
	"X08C": true, "X08E": true, "X8F": true, "X4B": true,
	"LX05": true, "OH2": true, "OH2P": true, "X6A": true,
}

// NeedPlayMusicAPI 硬件是否走 music API
func NeedPlayMusicAPI(hardware string) bool { return needUsePlayMusicAPI[hardware] }

// SpeakerAuth 认证管理器
type SpeakerAuth struct {
	mu        sync.Mutex
	account   string
	password  string
	client    *miservice.Client
	tokenPath string
	loggedIn  bool
}

// NewSpeakerAuth 构造；自动尝试恢复已有 token
func NewSpeakerAuth() *SpeakerAuth {
	home, _ := os.UserHomeDir()
	a := &SpeakerAuth{tokenPath: filepath.Join(home, ".config", "homecast", "speaker.json")}
	if t := a.restoreToken(); t != nil {
		client := miservice.NewClient("", "")
		client.Token = t
		a.client = client
		a.loggedIn = true
	}
	return a
}

// restoreToken 从持久化文件恢复 miservice.Tokens
func (a *SpeakerAuth) restoreToken() *miservice.Tokens {
	data, err := os.ReadFile(a.tokenPath)
	if err != nil {
		return nil
	}
	var t miservice.Tokens
	if json.Unmarshal(data, &t) != nil {
		return nil
	}
	if t.PassToken == "" || t.Sids["micoapi"].ServiceToken == "" {
		return nil
	}
	return &t
}

// saveToken 持久化 token
func (a *SpeakerAuth) saveToken() {
	if a.client == nil || a.client.Token == nil {
		return
	}
	data, err := json.MarshalIndent(a.client.Token, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(a.tokenPath), 0o755)
	_ = os.WriteFile(a.tokenPath, data, 0o600)
}

// IsLoggedIn 是否已登录
func (a *SpeakerAuth) IsLoggedIn() bool {
	return a.loggedIn && a.client != nil
}

// UserID 当前登录用户
func (a *SpeakerAuth) UserID() string {
	if a.client != nil && a.client.Token != nil {
		return a.client.Token.UserId
	}
	return ""
}

// Login 账号密码登录
func (a *SpeakerAuth) Login(account, password string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	client := miservice.NewClient(account, password)
	if err := client.Login("micoapi"); err != nil {
		return err
	}
	a.client = client
	a.loggedIn = true
	a.saveToken()
	log.Printf("[speaker] 账号登录成功: %s", account)
	return nil
}

// LoginWithToken 用已有 token（QR 登录产物）登录/刷新
func (a *SpeakerAuth) LoginWithToken(t *miservice.Tokens) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	client := miservice.NewClient("", "")
	client.Token = t
	if err := client.Login("micoapi"); err != nil {
		return err
	}
	a.client = client
	a.loggedIn = true
	a.saveToken()
	log.Printf("[speaker] token 登录成功: %s", t.UserId)
	return nil
}

// Logout 退出
func (a *SpeakerAuth) Logout() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.client = nil
	a.loggedIn = false
	_ = os.Remove(a.tokenPath)
}

// ListDevices 设备列表（miservice ListMinaDevices）
func (a *SpeakerAuth) ListDevices() ([]SpeakerDevice, error) {
	if a.client == nil {
		return nil, nil
	}
	devices, err := a.client.ListMinaDevices(0)
	if err != nil {
		return nil, err
	}
	out := make([]SpeakerDevice, 0, len(devices))
	for _, d := range devices {
		if d.DeviceID == "" || d.Hardware == "" || d.MiotDID == "" {
			continue
		}
		name := d.Name
		if name == "" {
			name = d.Alias
		}
		if name == "" {
			name = "未知设备"
		}
		out = append(out, SpeakerDevice{
			DeviceID: d.DeviceID,
			Hardware: d.Hardware,
			DID:      d.MiotDID,
			Name:     name,
			IsOnline: true,
		})
	}
	return out, nil
}

// PlayURL 播放 URL（deviceId 是设备 hardware ID）
func (a *SpeakerAuth) PlayURL(deviceID, url string) (any, error) {
	if a.client == nil {
		return nil, nil
	}
	return a.client.PlayerPlayUrl(deviceID, url)
}

// Pause 暂停
func (a *SpeakerAuth) Pause(deviceID string) (any, error) {
	if a.client == nil {
		return nil, nil
	}
	return a.client.PlayerPause(deviceID)
}

// Stop 停止
func (a *SpeakerAuth) Stop(deviceID string) (any, error) {
	if a.client == nil {
		return nil, nil
	}
	return a.client.PlayerStop(deviceID)
}

// Resume 恢复
func (a *SpeakerAuth) Resume(deviceID string) (any, error) {
	if a.client == nil {
		return nil, nil
	}
	return a.client.PlayerResume(deviceID)
}

// GetStatus 播放状态（对齐 Python：info JSON 解析 status/volume/loop_type）
func (a *SpeakerAuth) GetStatus(deviceID string) (map[string]any, error) {
	if a.client == nil {
		return nil, nil
	}
	v, err := a.client.PlayerGetStatus(deviceID)
	if err != nil {
		return nil, err
	}
	// 返回结构可能是 map，d.info 是 JSON 字符串
	out := map[string]any{"status": 0, "volume": 0, "loop_type": 0}
	if m, ok := v.(map[string]any); ok {
		if info, ok := m["info"].(string); ok && info != "" {
			var infoMap map[string]any
			if json.Unmarshal([]byte(info), &infoMap) == nil {
				for _, k := range []string{"status", "volume", "loop_type"} {
					if val, ok := infoMap[k]; ok {
						out[k] = val
					}
				}
			}
		}
	}
	return out, nil
}

// SetVolume 音量 0-100
func (a *SpeakerAuth) SetVolume(deviceID string, volume int) (any, error) {
	if a.client == nil {
		return nil, nil
	}
	if volume < 0 {
		volume = 0
	}
	if volume > 100 {
		volume = 100
	}
	return a.client.PlayerSetVolume(deviceID, volume)
}

// Tokens 暴露 token（QR 登录成功后注入用）
func (a *SpeakerAuth) Tokens() *miservice.Tokens {
	if a.client == nil || a.client.Token == nil {
		return nil
	}
	return a.client.Token
}