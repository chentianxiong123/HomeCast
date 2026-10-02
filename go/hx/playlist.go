// 播放列表功能切片（M2）：音乐页 = 播放列表 + 添加/移除/清空
// 数据走现有 JSON 服务（数据不分裂）；dock 队列由播放过的歌组成（player 孤岛内部态）
package hx

import (
	"net/http"

	"homecast/internal/service"
)

// playlistPageData 音乐页数据
type playlistPageData struct {
	Items []service.PlaylistItem
}

// MusicPage GET /hx/music → 播放列表页
func (h *H) MusicPage(w http.ResponseWriter, r *http.Request) {
	list := h.PL.Get().List
	h.renderPage(w, "music", "content_music.html", &playlistPageData{Items: list})
}

// AddPL POST /hx/playlist/add（bvid）→ 加播，返回按钮内提示
func (h *H) AddPL(w http.ResponseWriter, r *http.Request) {
	bvid := r.PostFormValue("bvid")
	if bvid == "" {
		http.Error(w, "bvid required", http.StatusBadRequest)
		return
	}
	if _, err := h.PL.Add(bvid); err != nil {
		http.Error(w, "添加失败："+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(`<span class="text-pink-400 text-sm font-medium">已加入 ✓</span>`))
}

// RemovePL POST /hx/playlist/remove/{bvid} → 返回列表片段替换 #pl-list
func (h *H) RemovePL(w http.ResponseWriter, r *http.Request) {
	bvid := r.PathValue("bvid")
	if bvid == "" {
		http.Error(w, "bvid required", http.StatusBadRequest)
		return
	}
	h.PL.Remove(bvid)
	h.renderPlaylistList(w)
}

// ClearPL POST /hx/playlist/clear → 清空并返回列表片段
func (h *H) ClearPL(w http.ResponseWriter, r *http.Request) {
	h.PL.Clear()
	h.renderPlaylistList(w)
}

// renderPlaylistList 渲染播放列表片段（共用零件）
func (h *H) renderPlaylistList(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	list := h.PL.Get().List
	h.Tpl.ExecuteTemplate(w, "playlist_list.html", &playlistPageData{Items: list})
}