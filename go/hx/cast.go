// 投屏功能切片（M4）：DLNA 设备发现 + 投送当前播放 + 状态
// 投送按钮只广播 hc:cast-play（孤岛不共享）→ player.js 持当前歌调这里投送
package hx

import (
	"log"
	"net/http"

	"homecast/internal/service"
)

// CastPage GET /hx/cast → 投屏页（设备列表 + 音箱区）
func (h *H) CastPage(w http.ResponseWriter, r *http.Request) {
	h.renderPage(w, "cast", "content_cast.html", &castPageData{Devices: h.Cast.DeviceList()})
}

// castPageData 投屏页数据
type castPageData struct {
	Devices []service.DeviceDTO
}

// Rescan POST /hx/cast/rescan → 重新发现设备，返回列表片段替换 #cast-list
func (h *H) Rescan(w http.ResponseWriter, r *http.Request) {
	h.Cast.Discover()
	h.renderCastList(w)
}

// CastPlay POST /hx/cast/play 表单 bvid,udn,title → 投送当前歌曲到 DLNA 设备
func (h *H) CastPlay(w http.ResponseWriter, r *http.Request) {
	bvid := r.PostFormValue("bvid")
	udn := r.PostFormValue("udn")
	title := r.PostFormValue("title")
	if bvid == "" || udn == "" {
		http.Error(w, "bvid and udn required", http.StatusBadRequest)
		return
	}
	if _, err := h.Cast.Cast("https://www.bilibili.com/video/"+bvid, udn, title); err != nil {
		log.Printf("[hx cast] %v", err)
		w.Write([]byte(`<span class="text-red-400 text-sm">投送失败</span>`))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(`<span class="text-green-400 text-sm">投送成功 ✓</span>`))
}

// CastStatus GET /hx/cast/status/{udn} → 设备播放状态片段
func (h *H) CastStatus(w http.ResponseWriter, r *http.Request) {
	udn := r.PathValue("udn")
	status, err := h.Cast.Status(udn)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err != nil || status == nil {
		w.Write([]byte(`<span class="text-gray-500 text-xs">状态不可用</span>`))
		return
	}
	state, _ := status["state"].(string)
	if state == "" {
		state = "就绪"
	}
	w.Write([]byte(`<span class="text-green-500 text-xs">` + state + `</span>`))
}

// renderCastList 设备列表片段（共用零件）
func (h *H) renderCastList(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.Tpl.ExecuteTemplate(w, "cast_devices.html", &castPageData{Devices: h.Cast.DeviceList()})
}