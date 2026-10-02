// Package web 前端资源工具箱：所有静态资源与模板 go:embed 内嵌进二进制
//
// 理念：无 CDN、无 Node、无构建链——htmx/alpine/daisyUI 全部本地化，
// 一个二进制自带全部页面。
package web

import (
	"embed"
	"html/template"
	"io/fs"
	"strings"
)

//go:embed assets
var assetsFS embed.FS

//go:embed templates/*.html
var templatesFS embed.FS

// Assets 静态资源（htmx/alpine/daisyUI/图片）
func Assets() fs.FS {
	return assetsFS
}

// Funcs 模板函数
func Funcs() template.FuncMap {
	return template.FuncMap{
		// 封面：//i0.hdslb.com/... → https://i0.hdslb.com/...
		"coverURL": func(c string) string {
			if c == "" {
				return ""
			}
			if strings.HasPrefix(c, "//") {
				return "https:" + c
			}
			return c
		},
		// 时长：秒 → mm:ss
		"fmtDur": func(sec int) template.HTML {
			m := sec / 60
			s := sec % 60
			return template.HTML("<span>" + itoa(m) + ":" + pad2(s) + "</span>")
		},
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func pad2(n int) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

// Templates 解析全部模板（页面 + htmx 片段），模板名 = 文件名（如 content_favs.html）
func Templates() (*template.Template, error) {
	tpl := template.New("").Funcs(Funcs())
	entries, err := templatesFS.ReadDir("templates")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".html") {
			continue
		}
		b, err := templatesFS.ReadFile("templates/" + e.Name())
		if err != nil {
			return nil, err
		}
		if _, err := tpl.New(e.Name()).Parse(string(b)); err != nil {
			return nil, err
		}
	}
	return tpl, nil
}

// MustTemplates 解析失败即 panic（模板是启动期资产）
func MustTemplates() *template.Template {
	return template.Must(Templates())
}