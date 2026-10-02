// Package dlna DLNA 设备发现与控制（goupnp 标准库）
package dlna

import (
	"context"
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/huin/goupnp/httpu"
	"github.com/huin/goupnp/ssdp"
)

// Device 投屏设备（对齐 Python DLNADevice）
type Device struct {
	UDN        string   `json:"udn"`
	Name       string   `json:"name"`
	Location   string   `json:"location"`
	IP         string   `json:"ip"`
	Port       int      `json:"port"`
	DeviceType string   `json:"device_type"`
}

var searchTypes = []string{
	"urn:schemas-upnp-org:device:MediaRenderer:1",
	"urn:schemas-upnp-org:service:AVTransport:1",
	"upnp:rootdevice",
}

// Search 多播搜索 DLNA 设备（对齐 Python SSDP 搜索语义）
func Search(ctx context.Context, timeout time.Duration, targetIP string) []*Device {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	httpuClient, err := httpu.NewHTTPUClient()
	if err != nil {
		log.Printf("[dlna] httpu client: %v", err)
		return nil
	}
	defer httpuClient.Close()

	seen := map[string]*Device{}
	var mu sync.Mutex

	for _, st := range searchTypes {
		responses, err := ssdp.RawSearch(ctx, httpuClient, st, 3)
		if err != nil {
			continue
		}
		for _, resp := range responses {
			dev := parseHTTPResponse(resp)
			if dev == nil {
				continue
			}
			mu.Lock()
			if _, dup := seen[dev.UDN]; !dup {
				seen[dev.UDN] = dev
			}
			mu.Unlock()
		}
	}

	// 补充 friendlyName（从描述 XML；Python 同步逻辑）
	for _, d := range seen {
		if n := fetchFriendlyName(d.Location); n != "" {
			d.Name = n
		}
		if d.Name == "" {
			d.Name = "Unknown"
		}
	}
	out := make([]*Device, 0, len(seen))
	for _, d := range seen {
		out = append(out, d)
	}
	log.Printf("[dlna] found %d devices", len(out))
	return out
}

func parseHTTPResponse(resp *http.Response) *Device {
	loc := resp.Header.Get("LOCATION")
	usn := resp.Header.Get("USN")
	st := resp.Header.Get("ST")
	if loc == "" || usn == "" {
		return nil
	}
	udn := usn
	if i := strings.Index(udn, "::"); i > 0 {
		udn = udn[:i]
	}
	ip, port := parseLocation(loc)
	return &Device{
		UDN:        strings.TrimSpace(udn),
		Location:   loc,
		IP:         ip,
		Port:       port,
		DeviceType: st,
		Name:       strings.Split(resp.Header.Get("SERVER"), "/")[0],
	}
}

func parseLocation(location string) (string, int) {
	u := location
	if strings.HasPrefix(u, "http://") {
		u = u[len("http://"):]
	}
	if i := strings.Index(u, "/"); i > 0 {
		u = u[:i]
	}
	parts := strings.Split(u, ":")
	ip := parts[0]
	port := 80
	if len(parts) > 1 {
		fmt.Sscanf(parts[1], "%d", &port)
	}
	return ip, port
}

// rootDevice 描述 XML（只取 friendlyName）
type rootDevice struct {
	XMLName     xml.Name `xml:"root"`
	FriendlyName string  `xml:"device>friendlyName"`
}

var descHC = &http.Client{Timeout: 3 * time.Second}

// fetchFriendlyName 拉设备描述 XML 取 friendlyName
func fetchFriendlyName(location string) string {
	if location == "" {
		return ""
	}
	req, err := http.NewRequest(http.MethodGet, location, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "PlayOn DLNA Controller")
	resp, err := descHC.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var rd rootDevice
	if err := xml.NewDecoder(resp.Body).Decode(&rd); err != nil {
		return ""
	}
	return strings.TrimSpace(rd.FriendlyName)
}

// DeviceByUDN 找设备
func DeviceByUDN(devices []*Device, udn string) *Device {
	for _, d := range devices {
		if d.UDN == udn {
			return d
		}
	}
	return nil
}