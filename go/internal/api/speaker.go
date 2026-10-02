package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"homecast/internal/service"
	"homecast/internal/speaker"

	"github.com/lsongdev/miservice-go/miservice"
)

// SpeakerHandler 音箱端点
type SpeakerHandler struct {
	Svc *service.SpeakerService
	QR  *speaker.QRLogin
}

// Status GET /api/v1/speaker/status
func (h *SpeakerHandler) Status(w http.ResponseWriter, r *http.Request) {
	isLoggedIn := h.Svc.Auth.IsLoggedIn()
	devices := h.Svc.ListDevices()
	ok(w, map[string]any{
		"is_logged_in": isLoggedIn,
		"device_count": len(devices),
		"devices":      devices,
	})
}

// Login POST /api/v1/speaker/login  body: {account, password, cookie}
func (h *SpeakerHandler) Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Account  string `json:"account"`
		Password string `json:"password"`
		Cookie   string `json:"cookie"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errResp(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Account == "" && body.Password == "" && body.Cookie == "" {
		errResp(w, http.StatusBadRequest, "请提供账号密码或 Cookie")
		return
	}
	var loginErr error
	if body.Cookie != "" {
		loginErr = h.loginViaCookie(body.Cookie)
	} else {
		loginErr = h.Svc.Auth.Login(body.Account, body.Password)
	}
	if loginErr != nil {
		log.Printf("[speaker login] %v", loginErr)
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"code": 401, "message": "登录失败: " + loginErr.Error(), "data": nil,
		})
		return
	}
	devices, _ := h.Svc.RefreshDevices()
	ok(w, map[string]any{
		"device_count": len(devices),
		"account":      body.Account,
		"devices":      devices,
	})
}

// loginViaCookie 用 cookie（userId/passToken[/deviceId]）登录
func (h *SpeakerHandler) loginViaCookie(cookie string) error {
	m := map[string]string{}
	for _, kv := range strings.Split(cookie, ";") {
		parts := strings.SplitN(strings.TrimSpace(kv), "=", 2)
		if len(parts) == 2 {
			m[parts[0]] = parts[1]
		}
	}
	t := miservice.NewTokens()
	t.PassToken = m["passToken"]
	t.UserId = m["userId"]
	t.DeviceId = m["deviceId"]
	if t.PassToken == "" || t.UserId == "" {
		return &appErr{"cookie 缺少 userId/passToken"}
	}
	return h.Svc.Auth.LoginWithToken(t)
}

type appErr struct{ s string }

func (e *appErr) Error() string { return e.s }

// Logout POST /api/v1/speaker/logout
func (h *SpeakerHandler) Logout(w http.ResponseWriter, r *http.Request) {
	h.Svc.Auth.Logout()
	h.QR.Reset()
	ok(w, true)
}

// Devices GET /api/v1/speaker/devices
func (h *SpeakerHandler) Devices(w http.ResponseWriter, r *http.Request) {
	ok(w, h.Svc.ListDevices())
}

// Play POST /api/v1/speaker/play  body: {bvid, did, quality}
func (h *SpeakerHandler) Play(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BVID    string `json:"bvid"`
		DID     string `json:"did"`
		Quality int    `json:"quality"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.BVID == "" || body.DID == "" {
		errResp(w, http.StatusBadRequest, "bvid and did required")
		return
	}
	if body.Quality == 0 {
		body.Quality = 30280 // 192k
	}
	res, err := h.Svc.Play(body.BVID, body.DID, body.Quality)
	if err != nil {
		log.Printf("[speaker play] %v", err)
	}
	writeJSON(w, http.StatusOK, res)
}

// Control POST /api/v1/speaker/control  body: {did, action, volume}
func (h *SpeakerHandler) Control(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DID    string `json:"did"`
		Action string `json:"action"`
		Volume int    `json:"volume"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.DID == "" {
		errResp(w, http.StatusBadRequest, "did required")
		return
	}
	res, err := h.Svc.Control(body.DID, body.Action, body.Volume)
	if err != nil {
		log.Printf("[speaker control] %v", err)
	}
	writeJSON(w, http.StatusOK, res)
}

// GetVolume GET /api/v1/speaker/volume/{did}
func (h *SpeakerHandler) GetVolume(w http.ResponseWriter, r *http.Request) {
	did := r.PathValue("did")
	dev, found := h.Svc.GetDevice(did)
	if !found {
		errResp(w, http.StatusNotFound, "设备未找到")
		return
	}
	status, err := h.Svc.Auth.GetStatus(dev.DeviceID)
	if err != nil || status == nil {
		errResp(w, http.StatusInternalServerError, "获取音量失败")
		return
	}
	ok(w, map[string]any{"volume": status["volume"]})
}

// SetVolume POST /api/v1/speaker/volume  body: {did, volume}
func (h *SpeakerHandler) SetVolume(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DID    string `json:"did"`
		Volume int    `json:"volume"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.DID == "" {
		errResp(w, http.StatusBadRequest, "did required")
		return
	}
	res, err := h.Svc.SetVolume(body.DID, body.Volume)
	if err != nil {
		log.Printf("[speaker volume] %v", err)
	}
	writeJSON(w, http.StatusOK, res)
}

// PlayerStatus GET /api/v1/speaker/player_status/{did}
func (h *SpeakerHandler) PlayerStatus(w http.ResponseWriter, r *http.Request) {
	did := r.PathValue("did")
	res, err := h.Svc.GetStatus(did)
	if err != nil {
		log.Printf("[speaker status] %v", err)
	}
	writeJSON(w, http.StatusOK, res)
}

// Refresh GET /api/v1/speaker/refresh
func (h *SpeakerHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	if !h.Svc.Auth.IsLoggedIn() {
		errResp(w, http.StatusUnauthorized, "未登录")
		return
	}
	devices, err := h.Svc.RefreshDevices()
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	ok(w, devices)
}

// QRGenerate POST /api/v1/speaker/qr/generate
func (h *SpeakerHandler) QRGenerate(w http.ResponseWriter, r *http.Request) {
	// 已有 token 先直接尝试登录
	if h.QR.IsLoggedIn() && !h.Svc.Auth.IsLoggedIn() {
		h.applyQRToken()
	}
	if h.Svc.Auth.IsLoggedIn() {
		ok(w, map[string]any{
			"is_logged_in": true, "user_id": h.Svc.Auth.UserID(),
			"qr_image": "", "status_url": "",
		})
		return
	}
	res, err := h.QR.GenerateQRCode()
	if err != nil {
		log.Printf("[speaker qr] %v", err)
		errResp(w, http.StatusInternalServerError, "生成二维码失败: "+err.Error())
		return
	}
	ok(w, res)
}

// QRStatus GET /api/v1/speaker/qr/status
func (h *SpeakerHandler) QRStatus(w http.ResponseWriter, r *http.Request) {
	res, err := h.QR.CheckStatus()
	if err != nil {
		errResp(w, http.StatusInternalServerError, err.Error())
		return
	}
	if res["status"] == "success" && !h.Svc.Auth.IsLoggedIn() {
		h.applyQRToken()
		devices, _ := h.Svc.RefreshDevices()
		res["device_count"] = len(devices)
	}
	ok(w, res)
}

// QRReset POST /api/v1/speaker/qr/reset
func (h *SpeakerHandler) QRReset(w http.ResponseWriter, r *http.Request) {
	h.QR.Reset()
	ok(w, true)
}

// applyQRToken QR 登录成功后用认证数据初始化 auth
func (h *SpeakerHandler) applyQRToken() {
	uid, pwd, ssec, stok, ok := h.QR.BuildTokens()
	if !ok {
		return
	}
	t := miservice.NewTokens()
	t.UserId = uid
	t.PassToken = pwd
	t.DeviceId = h.QR.DeviceID()
	t.Sids["micoapi"] = miservice.SidToken{Ssecurity: ssec, ServiceToken: stok}
	if err := h.Svc.Auth.LoginWithToken(t); err != nil {
		log.Printf("[speaker qr] login with token: %v", err)
	}
}

var _ = strconv.Itoa