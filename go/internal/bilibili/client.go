// Package bilibili 封装 B站 API 客户端（对齐 Python 版 client.py 的关键细节）。
package bilibili

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// BilibiliAPIError B站接口业务错误（code != 0）
type BilibiliAPIError struct {
	Code    int
	Message string
}

func (e *BilibiliAPIError) Error() string {
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// Client B站 API 客户端。并发安全：http.Client 可复用。
type Client struct {
	BaseURL string
	UA      string
	Referer string
	Timeout time.Duration
	buvid3  string
	hc      *http.Client
}

// NewClient 新建客户端；buvid3 每次随机（防风控，对齐 Python 版 uuid4）
func NewClient(baseURL, ua, referer string, timeout time.Duration) *Client {
	return &Client{
		BaseURL: baseURL,
		UA:      ua,
		Referer: referer,
		Timeout: timeout,
		buvid3:  newUUID(),
		hc: &http.Client{
			Timeout: timeout,
			// DASH 流代理需要跟随重定向（B站 baseUrl 302 到 CDN）
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return nil // 最多 10 次默认
			},
		},
	}
}

func newUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "00000000-0000-0000-0000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func (c *Client) defaultHeaders() http.Header {
	h := http.Header{}
	h.Set("User-Agent", c.UA)
	h.Set("Referer", c.Referer)
	h.Set("Origin", "https://search.bilibili.com")
	h.Set("Cookie", "buvid3="+c.buvid3)
	h.Set("Accept", "application/json, text/plain, */*")
	h.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	return h
}

// GetJSON 请求 JSON API，自动抛 code!=0。
func (c *Client) GetJSON(path string, params url.Values) (map[string]any, error) {
	u := c.BaseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header = c.defaultHeaders()
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bilibili http %d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	code, _ := body["code"].(float64)
	if int(code) != 0 {
		msg, _ := body["message"].(string)
		return nil, &BilibiliAPIError{Code: int(code), Message: msg}
	}
	data, _ := body["data"].(map[string]any)
	if data == nil {
		// data 可能是数组等其它形态，调用方自行解析
		return body, nil
	}
	return data, nil
}

// GetRaw 直接 GET 原始响应（流代理用：DASH 直转）。
func (c *Client) GetRaw(url string, extra http.Header) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	h := c.defaultHeaders()
	// 流直转必须 Referer: www.bilibili.com（B站 CDN 校验），与 JSON API 的 search.bilibili.com 不同
	h.Set("Referer", "https://www.bilibili.com")
	h.Set("Origin", "https://www.bilibili.com")
	for k, vv := range extra {
		for _, v := range vv {
			h.Add(k, v)
		}
	}
	req.Header = h
	return c.hc.Do(req)
}

// decodeData 通用：把已取出的 data 值塞进目标结构（GetJSON 已返回 data，直接 marshal）
func decodeData(v any, target any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, target)
}

var _ = io.Discard // 占位：io 可能后续用到
