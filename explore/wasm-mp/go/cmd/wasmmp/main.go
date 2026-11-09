//go:build js && wasm

// Package main 小程序 WASM 后端入口 + mp-html 渲染结合：
//   - Go 逻辑（搜 B站）编译 wasm 跑在小程序进程里，业务逻辑零改动
//   - 网络 http.Get 在 js/wasm 自动走宿主全局 fetch（Go net/http 内置 fetch 桥）
//   - Go 搜完把结果渲成 HTML 片段，用回调（js.FuncOf 传参）直接传回小程序
//   - 小程序用 mp-html 渲这个 HTML 片段（mp-html + wasm 结合：逻辑 in wasm，渲染 in mp-html）
//
// 交互模式（Go wasm 标准）：main 里 js.FuncOf 注册回调，网络放 goroutine（go.run 事件循环驱动），select{} 保活。
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"syscall/js"
)

// newBuvid B站防风控 cookie（对齐 internal/bilibili/client.go 每次随机；crypto/rand 在 js/wasm 挂，用 math/rand）
func newBuvid() string {
	b := make([]byte, 16)
	for i := range b {
		b[i] = byte(rand.Intn(256))
	}
	s := ""
	for _, x := range b {
		s += "0123456789abcdef"[x>>4:x>>4+1] + "0123456789abcdef"[x&15:x&15+1]
	}
	return s
}

// searchHTML 搜 B站 → 解析 → 渲成 HTML 卡片片段（mp-html 渲）
func searchHTML(kw string) string {
	u := "https://api.bilibili.com/x/web-interface/search/type?search_type=video&keyword=" +
		url.QueryEscape(kw) + "&page=1&duration=1&tids=3"
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36")
	req.Header.Set("Referer", "https://search.bilibili.com/")
	req.Header.Set("Cookie", "buvid3="+newBuvid())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return `<div style="color:#e33;padding:16px">搜索失败：` + err.Error() + `</div>`
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var d struct {
		Data struct {
			Result []struct {
				BVID     string `json:"bvid"`
				Title    string `json:"title"`
				Author   string `json:"author"`
				Duration string `json:"duration"`
				Pic      string `json:"pic"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &d); err != nil {
		return `<div style="color:#e33;padding:16px">解析失败：` + err.Error() + `</div>`
	}
	var b strings.Builder
	for _, it := range d.Data.Result {
		b.WriteString(fmt.Sprintf(
			`<div style="display:flex;gap:10px;padding:10px;background:#fff;border-radius:14px;margin:8px 0;box-shadow:0 1px 4px rgba(0,0,0,.06)">
  <a href="https://play.homecast/?bvid=%s" style="display:flex;gap:10px;text-decoration:none;color:#222;flex:1">
    <img src="%s" style="width:88px;height:88px;border-radius:10px;object-fit:cover;flex-shrink:0"/>
    <div style="flex:1;overflow:hidden">
      <div style="font-size:15px;font-weight:600;white-space:nowrap;overflow:hidden;text-overflow:ellipsis">%s</div>
      <div style="font-size:13px;color:#999;margin-top:6px">%s · %s</div>
    </div>
  </a>
  <div style="align-self:center;color:#fa2c19;font-size:18px">▶</div>
</div>`,
			it.BVID, it.Pic, it.Title, it.Author, it.Duration))
	}
	if b.Len() == 0 {
		return `<div style="color:#999;padding:16px;text-align:center">无结果</div>`
	}
	return b.String()
}

func main() {
	// 注册搜索回调：小程序调 __searchBili(kw, cb) → Go 事件循环执行 → goroutine 搜 B站 → 渲 HTML → cb(html)
	js.Global().Set("__searchBili", js.FuncOf(func(this js.Value, args []js.Value) any {
		kw := "周杰伦"
		if len(args) > 0 && args[0].Type() == js.TypeString && args[0].String() != "" {
			kw = args[0].String()
		}
		cb := js.Value{}
		if len(args) > 1 {
			cb = args[1]
		}
		go func() {
			html := searchHTML(kw)
			if !cb.IsUndefined() && !cb.IsNull() {
				cb.Invoke(html)
			}
		}()
		return nil
	}))
	// 保活：Go 运行时事件循环持续驱动（fetch 回调/goroutine 调度）
	select {}
}
