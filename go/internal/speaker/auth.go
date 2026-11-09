// Package speaker 小米小爱音箱（对齐 Python speaker/auth.py + device_manager.py）
//
// 登录方式：
//  1. 账号密码（miservice-go 原生）
//  2. 已有 token 恢复（~/.config/homecast/speaker.json，miservice.Tokens JSON）
//  3. 二维码登录（qrcode.go，走 account.xiaomi.com 长轮询）
package speaker

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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
	lastRefresh time.Time // 上次刷新 serviceToken 的时间（启动/登录/失败重刷时更新）
}

// NewSpeakerAuth 构造；自动尝试恢复已有 token
func NewSpeakerAuth() *SpeakerAuth {
	home, _ := os.UserHomeDir()
	a := &SpeakerAuth{tokenPath: filepath.Join(home, ".config", "homecast", "speaker.json")}
	if t := a.restoreToken(); t != nil {
		client := miservice.NewClient("", "")
		client.Token = t
		// 持久化 token 必须 Login("micoapi") 刷新 serviceToken：serviceToken 有效期短，
		// 直接使用会 401（device_list 空）。passToken 快路径无需密码。
		if err := client.Login("micoapi"); err == nil {
			a.client = client
			a.loggedIn = true
			a.lastRefresh = time.Now()
			a.saveToken()
			log.Printf("[speaker] 恢复持久化 token 并刷新登录态 (userId=%s)", t.UserId)
			return a
		} else {
			log.Printf("[speaker] 持久化 token 刷新失败（可能过期，尝试 QR 回退）: %v", err)
		}
	}
	// 回退：QR 扫码产物 speaker_auth.json（对齐 Python auth._set_token 优先读 auth.json）。
	// passToken+userId 在即可构造 token，但 QR 产物缺 serviceToken（设备列表认证必需），
	// 必须 Login("micoapi") 用已有 passToken 刷新（Python auth.login 同语义），成功才登录态成立。
	if t := restoreFromQR(filepath.Join(home, ".config", "homecast", "speaker_auth.json")); t != nil {
		client := miservice.NewClient("", "")
		client.Token = t
		if err := client.Login("micoapi"); err == nil {
			a.client = client
			a.loggedIn = true
			a.lastRefresh = time.Now()
			a.saveToken() // 刷新后的 token 落 speaker.json，下次走 restoreToken 快路径
			log.Printf("[speaker] 从 QR speaker_auth.json 恢复并刷新登录态 (userId=%s)", t.UserId)
		} else {
			log.Printf("[speaker] QR 恢复刷新失败（需重新扫码）: %v", err)
		}
	}
	return a
}

// restoreFromQR 从 QRLogin 扫码产物构造最小 token（换掉坏 payload 历史数据时用）
func restoreFromQR(path string) *miservice.Tokens {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(data, &m) != nil {
		return nil
	}
	pt, _ := m["passToken"].(string)
	uid, _ := m["userId"].(string)
	if pt == "" || uid == "" {
		return nil
	}
	if fixed, bad := fixSciNotation(uid); bad {
		uid = fixed
	}
	// deviceId 不在 QR 产物里，用随机占位（不影响 passToken 刷新）
	t := miservice.NewTokens()
	t.PassToken = pt
	t.UserId = uid
	t.DeviceId = fmt.Sprintf("%016x", time.Now().UnixNano())
	if ssec, ok := m["ssecurity"].(string); ok && ssec != "" {
		t.Sids["micoapi"] = miservice.SidToken{Ssecurity: ssec}
	}
	return t
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
// retryOnAuth 失败兜底重试：第一次调用若失败（可能 serviceToken 过期），
// 用 passToken 重刷一次再重试（仅一次，不循环）。平时零开销——不主动定时刷新，
// 只在「每次打开刷一次」（NewSpeakerAuth 启动 Login）的基础上，真过期时自动恢复。
func (a *SpeakerAuth) retryOnAuth(fn func() error) error {
	if a.client == nil || !a.loggedIn {
		return fn()
	}
	err := fn()
	if err == nil {
		return nil
	}
	// 失败 → 重刷一次 → 重试一次。重刷失败返回原始错误（由调用方上报）。
	a.mu.Lock()
	rerr := a.refreshServiceToken()
	a.mu.Unlock()
	if rerr != nil {
		log.Printf("[speaker] 失败后重刷 serviceToken 也失败: %v（原始错误: %v）", rerr, err)
		return err
	}
	a.saveToken()
	log.Printf("[speaker] 失败后重刷 serviceToken 并重试 (userId=%s)", a.client.Token.UserId)
	return fn()
}

// refreshServiceToken 用 passToken 重新走 serviceLogin + requestServiceToken 刷新 serviceToken。
// 不依赖 miservice.Login——其 loginResp.Nonce 是 int64，JSON 里 nonce 超 2^53（float64 精确上限），
// 反序列化丢精度导致 clientSign 签名错，小米返回 401（刷新失败但旧 token 仍有效）。
// 这里用 UseNumber 保精度，复刻 miservice 的 serviceLogin/requestServiceToken 两步。
func (a *SpeakerAuth) refreshServiceToken() error {
	c := a.client
	if c == nil || c.Token == nil {
		return fmt.Errorf("client/token nil")
	}
	// step1: pass/serviceLogin?sid=micoapi 用 passToken cookie（无需密码）
	cookies := []*http.Cookie{
		{Name: "sdkVersion", Value: "3.9"},
		{Name: "deviceId", Value: c.Token.DeviceId},
	}
	if c.Token.PassToken != "" {
		cookies = append(cookies, &http.Cookie{Name: "userId", Value: c.Token.UserId})
		cookies = append(cookies, &http.Cookie{Name: "passToken", Value: c.Token.PassToken})
	}
	loginURL := fmt.Sprintf("https://account.xiaomi.com/pass/serviceLogin?sid=micoapi&_json=true")
	req, _ := http.NewRequest(http.MethodGet, loginURL, nil)
	req.Header.Set("User-Agent", miservice.UA)
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	// 用独立干净 client（新 jar）：避免 c.Client 累积的旧 cookie（serviceToken 等）
	// 导致小米返回「已登录」形态（无 ssecurity/nonce，只有 psecurity）→ 刷新失败。
	refreshClient := &http.Client{}
	resp, err := refreshClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	// 解析：UseNumber 保 nonce/userId 精度（miservice 用 int64 会丢精度 → 签名错 → 401）
	var data struct {
		Code      int         `json:"code"`
		Location  string      `json:"location"`
		Ssecurity string      `json:"ssecurity"`
		Nonce     json.Number `json:"nonce"`
		PassToken string      `json:"passToken"`
		UserId    json.Number `json:"userId"`
	}
	dec := json.NewDecoder(strings.NewReader(strings.TrimPrefix(string(body), "&&&START&&&")))
	dec.UseNumber()
	if err := dec.Decode(&data); err != nil {
		return err
	}
	if data.Code != 0 {
		return fmt.Errorf("serviceLogin code=%d", data.Code)
	}
	if data.Location == "" || data.Ssecurity == "" || data.Nonce == "" {
		return fmt.Errorf("serviceLogin 缺字段: location=%q ssecurity=%q nonce=%q", data.Location, data.Ssecurity, data.Nonce)
	}
	// step2: requestServiceToken——clientSign = base64(sha1("nonce={nonce}&{ssecurity}"))，nonce 原样字符串
	nsec := fmt.Sprintf("nonce=%s&%s", data.Nonce, data.Ssecurity)
	sum := sha1.Sum([]byte(nsec))
	clientSign := base64.StdEncoding.EncodeToString(sum[:])
	stURL := fmt.Sprintf("%s&clientSign=%s", data.Location, url.QueryEscape(clientSign))
	req2, _ := http.NewRequest(http.MethodGet, stURL, nil)
	req2.Header.Set("User-Agent", miservice.UA)
	resp2, err := refreshClient.Do(req2)
	if err != nil {
		return err
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		return fmt.Errorf("requestServiceToken HTTP %d", resp2.StatusCode)
	}
	var st string
	for _, ck := range resp2.Cookies() {
		if ck.Name == "serviceToken" {
			st = ck.Value
			break
		}
	}
	if st == "" {
		return fmt.Errorf("serviceToken 未在 cookie")
	}
	// 更新 token：ssecurity + serviceToken + 新 passToken（小米可能轮换）
	if data.PassToken != "" {
		c.Token.PassToken = data.PassToken
	}
	if data.UserId != "" {
		c.Token.UserId = data.UserId.String()
	}
	if c.Token.Sids == nil {
		c.Token.Sids = map[string]miservice.SidToken{}
	}
	c.Token.Sids["micoapi"] = miservice.SidToken{
		Ssecurity:    data.Ssecurity,
		ServiceToken: st,
	}
	return nil
}

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
	a.lastRefresh = time.Now()
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
	a.lastRefresh = time.Now()
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
	var devices []miservice.DeviceData
	if err := a.retryOnAuth(func() error {
		var e error
		devices, e = a.client.ListMinaDevices(0)
		return e
	}); err != nil {
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
	var res any
	if err := a.retryOnAuth(func() error {
		var e error
		res, e = a.client.PlayerPlayUrl(deviceID, url)
		return e
	}); err != nil {
		return nil, err
	}
	return res, nil
}

// Pause 暂停
func (a *SpeakerAuth) Pause(deviceID string) (any, error) {
	if a.client == nil {
		return nil, nil
	}
	var res any
	if err := a.retryOnAuth(func() error {
		var e error
		res, e = a.client.PlayerPause(deviceID)
		return e
	}); err != nil {
		return nil, err
	}
	return res, nil
}

// Stop 停止
func (a *SpeakerAuth) Stop(deviceID string) (any, error) {
	if a.client == nil {
		return nil, nil
	}
	var res any
	if err := a.retryOnAuth(func() error {
		var e error
		res, e = a.client.PlayerStop(deviceID)
		return e
	}); err != nil {
		return nil, err
	}
	return res, nil
}

// Resume 恢复
func (a *SpeakerAuth) Resume(deviceID string) (any, error) {
	if a.client == nil {
		return nil, nil
	}
	var res any
	if err := a.retryOnAuth(func() error {
		var e error
		res, e = a.client.PlayerResume(deviceID)
		return e
	}); err != nil {
		return nil, err
	}
	return res, nil
}

// GetStatus 播放状态（对齐 Python：info JSON 解析 status/volume/loop_type）
func (a *SpeakerAuth) GetStatus(deviceID string) (map[string]any, error) {
	if a.client == nil {
		return nil, nil
	}
	var v any
	if err := a.retryOnAuth(func() error {
		var e error
		v, e = a.client.PlayerGetStatus(deviceID)
		return e
	}); err != nil {
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
	var res any
	if err := a.retryOnAuth(func() error {
		var e error
		res, e = a.client.PlayerSetVolume(deviceID, volume)
		return e
	}); err != nil {
		return nil, err
	}
	return res, nil
}

// Tokens 暴露 token（QR 登录成功后注入用）
func (a *SpeakerAuth) Tokens() *miservice.Tokens {
	if a.client == nil || a.client.Token == nil {
		return nil
	}
	return a.client.Token
}