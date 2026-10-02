package speaker

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	qrcode "github.com/skip2/go-qrcode"
)

// QRLogin 小米账号二维码登录（对齐 Python qrcode_login.py）
type QRLogin struct {
	mu         sync.Mutex
	deviceID   string
	locale     string
	status     string // idle/pending/scanning/success/failed
	message    string
	lpURL      string
	headers    map[string]string
	httpClient *http.Client

	authConfPath string
	authData     map[string]string
}

// NewQRLogin 构造
func NewQRLogin() *QRLogin {
	home, _ := os.UserHomeDir()
	q := &QRLogin{
		deviceID:     randomStr(16, "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_-"),
		locale:       "zh_CN",
		status:       "idle",
		authConfPath: filepath.Join(home, ".config", "homecast", "speaker_auth.json"),
		authData:     map[string]string{},
	}
	q.loadAuthData()
	return q
}

func randomStr(n int, charset string) string {
	b := make([]byte, n)
	for i := range b {
		idx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		b[i] = charset[idx.Int64()]
	}
	return string(b)
}

// 对齐 Python _generate_user_agent
func genUserAgent() string {
	return "Android-15-11.0.701-Xiaomi-23046RP50C-OS2.0.212.0.VMYCNXM-" +
		randomStr(40, "0123456789ABCDEF") + "-CN-" +
		randomStr(32, "0123456789ABCDEF") + "-" +
		randomStr(32, "0123456789ABCDEF") + "-SmartHome-MI_APP_STORE-" +
		randomStr(40, "0123456789ABCDEF") + "|" + randomStr(40, "0123456789ABCDEF") + "|test-64"
}

func (q *QRLogin) loadAuthData() {
	data, err := os.ReadFile(q.authConfPath)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &q.authData)
}

func (q *QRLogin) saveAuthData() {
	data, _ := json.Marshal(q.authData)
	_ = os.MkdirAll(filepath.Dir(q.authConfPath), 0o755)
	_ = os.WriteFile(q.authConfPath, data, 0o600)
}

func (q *QRLogin) getSession() *http.Client {
	if q.httpClient == nil {
		jar, _ := cookiejar.New(nil)
		q.httpClient = &http.Client{Jar: jar}
	}
	return q.httpClient
}

func (q *QRLogin) doGet(client *http.Client, u string, headers map[string]string, timeout time.Duration) (string, error) {
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if timeout > 0 {
		client.Timeout = timeout
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// stripStart 去掉 "&&&START&&&" 前缀
func stripStart(text string) string {
	return strings.TrimPrefix(text, "&&&START&&&")
}

// GenerateQRCode 生成二维码（返回 map：qr_url/qr_image/status_url）
func (q *QRLogin) GenerateQRCode() (map[string]any, error) {
	client := q.getSession()
	headers := map[string]string{
		"User-Agent":    genUserAgent(),
		"Connection":    "keep-alive",
		"Content-Type":  "application/x-www-form-urlencoded",
	}

	serviceLoginURL := fmt.Sprintf("https://account.xiaomi.com/pass/serviceLogin?_json=true&sid=mijia&_locale=%s", q.locale)
	if pt := q.authData["passToken"]; pt != "" {
		headers["Cookie"] = fmt.Sprintf("deviceId=%s; passToken=%s; userId=%s", q.deviceID, pt, q.authData["userId"])
	}
	text, err := q.doGet(client, serviceLoginURL, headers, 30*time.Second)
	if err != nil {
		return nil, err
	}
	var serviceData map[string]any
	if err := json.Unmarshal([]byte(stripStart(text)), &serviceData); err != nil {
		return nil, err
	}

	// token 有效直接成功
	if code, _ := serviceData["code"].(float64); code == 0 {
		if loc, _ := serviceData["location"].(string); loc != "" {
			if t, err := q.doGet(client, loc, headers, 30*time.Second); err == nil && t == "ok" {
				q.status = "success"
				q.message = "Token 有效，无需重新登录"
				if ssecurity, _ := serviceData["ssecurity"].(string); ssecurity != "" {
					q.authData["ssecurity"] = ssecurity
					q.saveAuthData()
				}
				return map[string]any{
					"success": true, "message": q.message,
					"qr_url": "", "qr_image": "", "status_url": "", "is_logged_in": true,
					"user_id": q.authData["userId"],
				}, nil
			}
		}
	}

	// 解析 location 参数 → 二维码长轮询 URL
	loc, _ := serviceData["location"].(string)
	u, err := url.Parse(loc)
	if err != nil {
		return nil, err
	}
	qs, _ := url.ParseQuery(u.RawQuery)
	locData := url.Values{}
	for k, v := range qs {
		locData.Set(k, v[0])
	}
	locData.Set("theme", "")
	locData.Set("bizDeviceType", "")
	locData.Set("_hasLogo", "false")
	locData.Set("_qrsize", "240")
	locData.Set("_dc", strconv.FormatInt(time.Now().UnixMilli(), 10))

	loginURL := "https://account.xiaomi.com/longPolling/loginUrl?" + locData.Encode()
	t2, err := q.doGet(client, loginURL, headers, 30*time.Second)
	if err != nil {
		return nil, err
	}
	var loginData map[string]any
	if err := json.Unmarshal([]byte(stripStart(t2)), &loginData); err != nil {
		return nil, err
	}
	qrURL, _ := loginData["loginUrl"].(string)
	lp, _ := loginData["lp"].(string)
	if qrURL == "" {
		return nil, fmt.Errorf("二维码生成失败: %v", loginData)
	}

	// 生成二维码 PNG base64
	png, err := qrcode.Encode(qrURL, qrcode.Medium, 240)
	if err != nil {
		return nil, err
	}

	q.status = "pending"
	q.message = "等待扫码"
	q.lpURL = lp
	q.headers = headers

	return map[string]any{
		"success": true,
		"message": "请使用米家 APP 扫描二维码",
		"qr_url":  qrURL,
		"qr_image": "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
		"status_url": lp,
		"is_logged_in": false,
	}, nil
}

// CheckStatus 轮询登录状态（长轮询期间不持锁，避免阻塞其它端点）
func (q *QRLogin) CheckStatus() (map[string]any, error) {
	q.mu.Lock()
	if q.status == "idle" || q.status == "success" || q.status == "failed" {
		s := q.status
		m := q.message
		uid := q.authData["userId"]
		q.mu.Unlock()
		return map[string]any{"status": s, "message": m, "user_id": uid}, nil
	}
	lpURL := q.lpURL
	headers := q.headers
	q.mu.Unlock()
	if lpURL == "" {
		return map[string]any{"status": "failed", "message": "未生成二维码", "user_id": ""}, nil
	}

	// 独立 client 长轮询（120s 等扫码；Timeout 只作用于本请求）
	client := &http.Client{Timeout: 120 * time.Second}
	text, err := q.doGet(client, lpURL, headers, 120*time.Second)
	if err != nil {
		q.mu.Lock()
		q.status = "failed"
		q.message = "登录失败: " + err.Error()
		q.mu.Unlock()
		return map[string]any{"status": "failed", "message": err.Error(), "user_id": ""}, nil
	}
	var lpData map[string]any
	if err := json.Unmarshal([]byte(stripStart(text)), &lpData); err != nil {
		q.mu.Lock()
		q.status = "failed"
		q.mu.Unlock()
		return map[string]any{"status": "failed", "message": "解析失败", "user_id": ""}, nil
	}
	if code, _ := lpData["code"].(float64); code == 0 {
		for _, key := range []string{"psecurity", "nonce", "ssecurity", "passToken", "userId", "cUserId", "serviceToken"} {
			if v, ok := lpData[key]; ok {
				q.authData[key] = fmt.Sprint(v)
			}
		}
		q.saveAuthData()
		q.mu.Lock()
		q.status = "success"
		q.message = "登录成功"
		q.lpURL = ""
		q.mu.Unlock()
		return map[string]any{"status": "success", "message": "登录成功", "user_id": q.authData["userId"]}, nil
	}
	q.mu.Lock()
	q.status = "pending"
	q.mu.Unlock()
	return map[string]any{"status": "pending", "message": "等待扫码...", "user_id": ""}, nil
}

// CookieString userId/passToken/deviceId 拼接（给 auth 用）
func (q *QRLogin) CookieString() string {
	return fmt.Sprintf("deviceId=%s; passToken=%s; userId=%s", q.deviceID, q.authData["passToken"], q.authData["userId"])
}

// AuthData 登录成功后的认证数据（用户构造 miservice.Tokens）
func (q *QRLogin) AuthData() map[string]string {
	return q.authData
}

// DeviceID 登录设备 ID
func (q *QRLogin) DeviceID() string { return q.deviceID }

// UserID 已登录用户
func (q *QRLogin) UserID() string { return q.authData["userId"] }

// IsLoggedIn 是否已登录
func (q *QRLogin) IsLoggedIn() bool { return q.status == "success" && q.authData["passToken"] != "" }

// Reset 重置
func (q *QRLogin) Reset() {
	q.status = "idle"
	q.message = ""
	q.lpURL = ""
	q.headers = nil
	if q.httpClient != nil {
		q.httpClient.CloseIdleConnections()
		q.httpClient = nil
	}
}

// BuildTokens 把 QR 认证数据转成 miservice.Tokens（供 auth.LoginWithToken）
func (q *QRLogin) BuildTokens() (userID, passToken, ssecurity, serviceToken string, ok bool) {
	userID = q.authData["userId"]
	passToken = q.authData["passToken"]
	ssecurity = q.authData["ssecurity"]
	serviceToken = q.authData["serviceToken"]
	ok = userID != "" && passToken != "" && serviceToken != ""
	return
}