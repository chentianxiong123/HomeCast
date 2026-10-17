// 音箱功能切片（M4b）：小爱音箱——扫码登录 / 设备 / 控制 / 投送
// 登录态判定 → 渲染登录 or 面板；QR 扫码后 CheckStatus 挂起轮询直至成功
package hx

import (
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/lsongdev/miservice-go/miservice"

	spk "homecast/internal/service/speaker"
	"homecast/internal/speaker"
)

// atoiSafe 表单整数字段容错（面向过程零件）
func atoiSafe(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// SpeakerSection GET /hx/speaker → 音箱区片段（按登录态渲染）
func (h *H) SpeakerSection(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if !h.Speaker.Auth.IsLoggedIn() {
		h.Tpl.ExecuteTemplate(w, "speaker_login.html", nil)
		return
	}
	devices, err := h.Speaker.RefreshDevices()
	if err != nil {
		devices = h.Speaker.ListDevices()
	}
	h.Tpl.ExecuteTemplate(w, "speaker_panel.html", &speakerPanelData{
		UserID:  h.Speaker.Auth.UserID(),
		Devices: devices,
	})
}

// speakerPanelData 已登录面板数据
type speakerPanelData struct {
	UserID  string
	Devices []speaker.SpeakerDevice
}

// SpeakerQRStart POST /hx/speaker/qr-start → 生成二维码片段（图 + 挂起轮询）
func (h *H) SpeakerQRStart(w http.ResponseWriter, r *http.Request) {
	m, err := h.QR.GenerateQRCode()
	if err != nil {
		log.Printf("[speaker qr] %v", err)
		http.Error(w, "二维码生成失败："+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	img, _ := m["qr_image"].(string)
	// template.URL：img 是 data:image/png;base64，普通 string 会被 html/template
	// 当不安全 URL scheme 替换成 #ZgotmplZ（图片死链）；template.URL 是白名单类型
	h.Tpl.ExecuteTemplate(w, "speaker_qr.html", map[string]any{"QRImage": template.URL(img)})
}

// SpeakerQRStatus GET /hx/speaker/qr-status → CheckStatus 挂起（最长 120s）等待扫码
// 成功 → 登录并刷新音箱区为面板；失败/空闲 → 提示（前端可重新扫码）
func (h *H) SpeakerQRStatus(w http.ResponseWriter, r *http.Request) {
	m, err := h.QR.CheckStatus()
	if err != nil {
		log.Printf("[speaker qr status] %v", err)
		h.Tpl.ExecuteTemplate(w, "speaker_login.html", nil)
		return
	}
	status, _ := m["status"].(string)
	if status == "success" {
		h.applyQRToken()
		h.SpeakerSection(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	msg, _ := m["message"].(string)
	w.Write([]byte(`<p class="text-red-400 text-sm text-center mt-4">扫码未完成：` + msg + `</p>` +
		`<div class="text-center mt-3"><button hx-post="/hx/speaker/qr-start" hx-target="#sp-qr-box" hx-swap="innerHTML" class="px-4 py-1.5 rounded-full text-sm text-gray-400 border border-gray-600 hover:bg-gray-800 transition-colors">重新生成二维码</button></div>`))
}

// SpeakerPlay POST /hx/speaker/play 表单 bvid,did → 音箱播放
func (h *H) SpeakerPlay(w http.ResponseWriter, r *http.Request) {
	bvid := r.PostFormValue("bvid")
	did := r.PostFormValue("did")
	if bvid == "" || did == "" {
		http.Error(w, "bvid and did required", http.StatusBadRequest)
		return
	}
	duration, _ := strconv.ParseFloat(r.PostFormValue("duration"), 64)
	res, err := h.Speaker.Play(bvid, did, 30280, spk.PlayOpts{
		Title:    r.PostFormValue("title"),
		Duration: duration,
		NextBVID: r.PostFormValue("next_bvid"),
	})
	log.Printf("[speaker play] bvid=%s did=%s err=%v res=%v", bvid, did, err, res)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err != nil {
		w.Write([]byte(`<span class="text-red-400 text-sm">播放失败</span>`))
		return
	}
	if !speakerOK(res, &w) {
		return
	}
	w.Write([]byte(`<span class="text-green-400 text-sm">正在音箱播放 ✓</span>`))
}

// speakerOK 检查 service 返回的 {code,message}；code!=0 时向 w 写失败原因并返回 false。
// service 层对“设备未找到/音频流失败”这类软失败返回 err=nil + code!=0，
// handler 只判 err 会把失败当成功（绿色对勾），必须同时查 code。
func speakerOK(res map[string]any, w *http.ResponseWriter) bool {
	if res == nil {
		return true
	}
	if c, ok := res["code"].(int); ok && c != 0 {
		msg, _ := res["message"].(string)
		if msg == "" {
			msg = "未知错误"
		}
		log.Printf("[speaker] 软失败 code=%d message=%s", c, msg)
		(*w).Write([]byte(`<span class="text-red-400 text-sm">` + htmlEscape(msg) + `</span>`))
		return false
	}
	return true
}

func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;").Replace(s)
}

// SpeakerControl POST /hx/speaker/control 表单 did,action(play/pause/previous/next),volume
func (h *H) SpeakerControl(w http.ResponseWriter, r *http.Request) {
	did := r.PostFormValue("did")
	action := r.PostFormValue("action")
	vol := 0
	if action == "setvolume" {
		vol = atoiSafe(r.PostFormValue("volume"))
	}
	if did == "" || action == "" {
		http.Error(w, "did and action required", http.StatusBadRequest)
		return
	}
	// Control 对“设备未找到/未知操作/无已保存 URL”返回 err=nil + code!=0（软失败），
	// 必须同时查 code，否则界面会显示“✓ 操作成功”而实际未生效。
	if res, err := h.Speaker.Control(did, action, vol); err != nil {
		log.Printf("[speaker control] %v", err)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<span class="text-red-400 text-xs">操作失败</span>`))
		return
	} else if !speakerOK(res, &w) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(`<span class="text-green-500 text-xs">✓</span>`))
}

// SpeakerRefresh POST /hx/speaker/refresh → 重扫设备，返回设备列表片段
func (h *H) SpeakerRefresh(w http.ResponseWriter, r *http.Request) {
	devices, err := h.Speaker.RefreshDevices()
	if err != nil {
		devices = h.Speaker.ListDevices()
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.Tpl.ExecuteTemplate(w, "speaker_devices.html", &speakerPanelData{Devices: devices})
}

// applyQRToken QR token → Auth 登录（与 api/speaker.go 同逻辑）
func (h *H) applyQRToken() {
	uid, pwd, ssec, stok, ok := h.QR.BuildTokens()
	if !ok {
		return
	}
	t := miservice.NewTokens()
	t.UserId = uid
	t.PassToken = pwd
	t.DeviceId = h.QR.DeviceID()
	t.Sids["micoapi"] = miservice.SidToken{Ssecurity: ssec, ServiceToken: stok}
	if err := h.Speaker.Auth.LoginWithToken(t); err != nil {
		log.Printf("[speaker qr] login with token: %v", err)
	}
}