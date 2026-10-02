// Package cast 投屏服务（DLNA，goupnp 标准库控制）
package service

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/huin/goupnp/dcps/av1"

	"homecast/internal/bilibili"
	"homecast/internal/dlna"
)

// Episode 集条目（对齐 Python VideoEpisode JSON）
type Episode struct {
	Index     int    `json:"index"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Duration  int    `json:"duration,omitempty"`
	Thumbnail string `json:"thumbnail,omitempty"`
}

// SniffResult 嗅探返回
type SniffResult struct {
	Title        string    `json:"title"`
	Episodes     []Episode `json:"episodes"`
	EpisodesList []Episode `json:"episodes_list"`
	SniffMethod  string    `json:"sniff_method"`
}

var (
	biliPattern = regexp.MustCompile(`(?i)(bilibili\.com|b23\.tv)`)
	bvidPattern = regexp.MustCompile(`BV[a-zA-Z0-9]+`)
	videoExts   = []string{".mp4", ".webm", ".mkv", ".avi", ".flv", ".ts", ".mov", ".mpd", ".m3u8"}
)

// deviceController 单个 DLNA 设备的 UPnP 控制句柄
type deviceController struct {
	av *av1.AVTransport1
	rc *av1.RenderingControl1
}

// CastService 投屏服务
type CastService struct {
	client      *bilibili.Client
	tokens      *TokenStore
	mu          sync.Mutex
	devices     map[string]*dlna.Device
	controllers map[string]*deviceController
}

// NewCastService 构造
func NewCastService(client *bilibili.Client, tokens *TokenStore) *CastService {
	return &CastService{
		client:      client,
		tokens:      tokens,
		devices:     map[string]*dlna.Device{},
		controllers: map[string]*deviceController{},
	}
}

// Discover 发现 DLNA 设备（SSDP）
func (c *CastService) Discover() []*dlna.Device {
	devices := dlna.Search(context.Background(), 5*time.Second, "")
	c.mu.Lock()
	c.devices = map[string]*dlna.Device{}
	for _, d := range devices {
		c.devices[d.UDN] = d
	}
	c.mu.Unlock()
	return devices
}

type deviceDTO struct {
	Name       string `json:"name"`
	UDN        string `json:"udn"`
	IP         string `json:"ip"`
	Port       int    `json:"port"`
	DeviceType string `json:"device_type"`
}

// DeviceList 设备列表 DTO
func (c *CastService) DeviceList() []deviceDTO {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]deviceDTO, 0, len(c.devices))
	for _, d := range c.devices {
		out = append(out, deviceDTO{Name: d.Name, UDN: d.UDN, IP: d.IP, Port: d.Port, DeviceType: d.DeviceType})
	}
	return out
}

// Sniff 嗅探（降级版：B站 API + 直链识别；yt-dlp/浏览器已砍）
func (c *CastService) Sniff(urlStr string) *SniffResult {
	if biliPattern.MatchString(urlStr) {
		if res := c.sniffBilibili(urlStr); res != nil {
			return res
		}
	}
	// 直链视频
	ext := strings.ToLower(urlStr)
	for _, e := range videoExts {
		if strings.Contains(ext, e) {
			return &SniffResult{
				Title:    "视频",
				Episodes: []Episode{{Index: 1, Title: "视频", URL: urlStr}},
				SniffMethod: "direct",
			}
		}
	}
	return &SniffResult{Title: "", Episodes: nil}
}

// sniffBilibili B站详情/播放页 → 集列表（对齐 Python _try_bilibili_api）
func (c *CastService) sniffBilibili(urlStr string) *SniffResult {
	m := bvidPattern.FindString(urlStr)
	if m == "" {
		return nil
	}
	info, err := c.client.GetVideoInfo(m)
	if err != nil {
		return nil
	}
	eps := []Episode{{
		Index:     1,
		Title:     info.Title,
		URL:       "https://www.bilibili.com/video/" + m,
		Duration:  info.Duration,
		Thumbnail: info.Pic,
	}}
	return &SniffResult{Title: info.Title, Episodes: eps, SniffMethod: "bilibili-api"}
}

func isDirectVideoURL(u string) bool {
	p := strings.ToLower(u)
	for _, e := range videoExts {
		if strings.Contains(p, e) {
			return true
		}
	}
	return false
}

func guessContentType(u string) string {
	p := strings.ToLower(u)
	switch {
	case strings.Contains(p, ".m3u8"):
		return "application/x-mpegurl"
	case strings.Contains(p, ".mpd"):
		return "application/dash+xml"
	case strings.Contains(p, ".ts"):
		return "video/mp2t"
	case strings.Contains(p, ".flv"):
		return "video/x-flv"
	case strings.Contains(p, ".webm"):
		return "video/webm"
	}
	return "video/mp4"
}

func extractReferer(u string) string {
	if i := strings.Index(u, "://"); i > 0 {
		rest := u[i+3:]
		if j := strings.Index(rest, "/"); j > 0 {
			return u[:i+3+j] + "/"
		}
		return u
	}
	return ""
}

// PlayURL 取可播 URL：m3u8 直返；其他走 token 代理（带 referer）
func (c *CastService) PlayURL(videoURL, title string) (map[string]any, string) {
	// 解析出真实流地址（B站页面 → DASH 视频流）
	streamURL, ctype, referer := c.resolveStream(videoURL)
	if streamURL == "" {
		return map[string]any{"code": 500, "message": "不支持的视频格式"}, ""
	}
	if strings.Contains(strings.ToLower(streamURL), ".m3u8") {
		return map[string]any{"code": 0, "message": "success", "data": map[string]any{
			"url": streamURL, "type": "application/x-mpegurl", "hls": true,
		}}, streamURL
	}
	token := c.tokens.Store(streamURL, map[string]string{
		"content_type": ctype,
		"original_url": streamURL,
		"referer":      referer,
	})
	proxyURL := "/api/v1/proxy/video/" + token
	_ = title
	return map[string]any{"code": 0, "message": "success", "data": map[string]any{
		"url": proxyURL, "type": ctype, "hls": false,
	}}, proxyURL
}

// resolveStream 输入（页面/直链）→ 可播流地址 + content-type + referer
func (c *CastService) resolveStream(input string) (streamURL, ctype, referer string) {
	if isDirectVideoURL(input) {
		return input, guessContentType(input), extractReferer(input)
	}
	if biliPattern.MatchString(input) {
		m := bvidPattern.FindString(input)
		if m != "" {
			if info, err := c.client.GetVideoInfo(m); err == nil {
				if v, err := c.client.GetBestVideoURL(m, info.CID); err == nil {
					return v.BaseURL, v.MimeType, "https://www.bilibili.com/"
				}
			}
		}
	}
	return "", "", ""
}

// buildDIDLLite 投屏元数据（对齐 Python build_didl_lite）
func buildDIDLLite(uri, title, contentType string) string {
	upnpClass := "object.item.videoItem"
	if strings.HasPrefix(contentType, "audio") {
		upnpClass = "object.item.audioItem"
	}
	mime := contentType
	if mime == "" {
		mime = "video/mp4"
	}
	proto := "http-get:*:" + mime + ":DLNA.ORG_PN=;DLNA.ORG_OP=01;DLNA.ORG_CI=0;DLNA.ORG_FLAGS=01700000000000000000000000000000"
	escTitle := escapeXML(title)
	escURI := escapeXML(uri)
	meta := `<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dlna="urn:schemas-dlna-org:metadata-1-0/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/"><item id="0" parentID="-1" restricted="1">` +
		`<upnp:class>` + upnpClass + `</upnp:class><dc:title>` + escTitle + `</dc:title>` +
		`<res protocolInfo="` + proto + `">` + escURI + `</res></item></DIDL-Lite>`
	return meta
}

func escapeXML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

// controller 获取/创建设备控制句柄
func (c *CastService) controller(udn string) (*deviceController, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if dc, ok := c.controllers[udn]; ok {
		return dc, nil
	}
	dev, ok := c.devices[udn]
	if !ok {
		return nil, fmt.Errorf("device not found: %s", udn)
	}
	u, err := url.Parse(dev.Location)
	if err != nil {
		return nil, err
	}
	avs, err := av1.NewAVTransport1ClientsByURL(u)
	if err != nil || len(avs) == 0 {
		return nil, fmt.Errorf("no AVTransport service: %v", err)
	}
	dc := &deviceController{av: avs[0]}
	if rcs, err := av1.NewRenderingControl1ClientsByURL(u); err == nil && len(rcs) > 0 {
		dc.rc = rcs[0]
	}
	c.controllers[udn] = dc
	return dc, nil
}

// Cast 投屏：设置播放地址 + 开始播放
func (c *CastService) Cast(episodeURL, udn, title string) (map[string]any, error) {
	dc, err := c.controller(udn)
	if err != nil {
		return map[string]any{"code": 404, "message": err.Error()}, err
	}
	res, streamURL := c.PlayURL(episodeURL, title)
	if code, _ := res["code"].(int); code != 0 {
		return res, nil
	}
	data := res["data"].(map[string]any)
	ctype, _ := data["type"].(string)
	if err := dc.av.SetAVTransportURI(0, streamURL, buildDIDLLite(streamURL, title, ctype)); err != nil {
		return map[string]any{"code": 500, "message": "设置播放地址失败"}, err
	}
	if err := dc.av.Play(0, "1"); err != nil {
		return map[string]any{"code": 500, "message": "播放启动失败"}, err
	}
	return map[string]any{"code": 0, "message": "success", "data": map[string]any{"device_name": c.deviceName(udn)}}, nil
}

func (c *CastService) deviceName(udn string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if d, ok := c.devices[udn]; ok {
		return d.Name
	}
	return udn
}

// Control 控制：play/pause/stop/seek:mm:ss/volume:N
func (c *CastService) Control(udn, action, target string, volume int) (map[string]any, error) {
	dc, err := c.controller(udn)
	if err != nil {
		return map[string]any{"code": 404, "message": err.Error()}, err
	}
	var cerr error
	switch {
	case action == "play":
		cerr = dc.av.Play(0, "1")
	case action == "pause":
		cerr = dc.av.Pause(0)
	case action == "stop":
		cerr = dc.av.Stop(0)
	case action == "seek" && target != "":
		cerr = dc.av.Seek(0, "REL_TIME", target)
	case action == "volume" && volume >= 0:
		if dc.rc != nil {
			cerr = dc.rc.SetVolume(0, "Master", uint16(volume))
		} else {
			return map[string]any{"code": 500, "message": "设备不支持音量控制"}, nil
		}
	default:
		return map[string]any{"code": 400, "message": "未知操作: " + action}, nil
	}
	if cerr != nil {
		return map[string]any{"code": 500, "message": "操作失败: " + cerr.Error()}, cerr
	}
	return map[string]any{"code": 0, "message": "success", "data": true}, nil
}

// Status 设备状态
func (c *CastService) Status(udn string) (map[string]any, error) {
	dc, err := c.controller(udn)
	if err != nil {
		return map[string]any{"code": 404, "message": err.Error()}, err
	}
	state, status, _, err := dc.av.GetTransportInfo(0)
	if err != nil {
		return map[string]any{"code": 500, "message": err.Error()}, err
	}
	pos := map[string]any{"state": state, "status": status}
	if _, trackDur, _, trackURI, relTime, _, _, _, err := dc.av.GetPositionInfo(0); err == nil {
		pos["duration"] = trackDur
		pos["uri"] = trackURI
		pos["position"] = relTime
	}
	if dc.rc != nil {
		if v, err := dc.rc.GetVolume(0, "Master"); err == nil {
			pos["volume"] = v
		}
	}
	return map[string]any{"code": 0, "message": "success", "data": map[string]any{
		"transport": pos,
	}}, nil
}