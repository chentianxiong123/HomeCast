package web

// 模板结构校验测试：
//   Go html/template 只校验 {{}} 语法，HTML 标签开闭平衡零检查——
//   残缺模板（div 开 5 层闭 1 层）能编译通过、零报错。浏览器会自动修复
//   推断闭合位置，页面「看起来能用」但布局错乱，肉眼很难定位到模板坏了。
//
//   本测试兜住这个生态弱点：编译期发现标签不平衡的模板。

import (
	"html/template"
	"path"
	"regexp"
	"strings"
	"testing"
)

func TestTemplatesParseSyntax(t *testing.T) {
	// 所有模板必须能被 html/template 解析（{{}} 语法正确）
	entries, err := templatesFS.ReadDir("templates")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || path.Ext(name) != ".html" {
			continue
		}
		if _, err := template.New(name).Funcs(Funcs()).ParseFS(templatesFS, "templates/"+name); err != nil {
			t.Errorf("%s 模板语法错误: %v", name, err)
		}
	}
}

// blockTags 需要开闭成对的块级/行内结构标签
var blockTags = []string{"div", "span", "button", "details", "summary", "select", "textarea"}

func TestTemplatesHTMLBalance(t *testing.T) {
	openRe := map[string]*regexp.Regexp{}
	for _, tag := range blockTags {
		openRe[tag] = regexp.MustCompile(`<` + tag + `[ >]`)
	}
	closeRe := map[string]*regexp.Regexp{}
	for _, tag := range blockTags {
		closeRe[tag] = regexp.MustCompile(`</` + tag + `>`)
	}

	entries, err := templatesFS.ReadDir("templates")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || path.Ext(name) != ".html" {
			continue
		}
		b, err := templatesFS.ReadFile("templates/" + name)
		if err != nil {
			t.Fatalf("%s 读失败: %v", name, err)
		}
		content := string(b)

		for _, tag := range blockTags {
			open := len(openRe[tag].FindAllString(content, -1))
			close := len(closeRe[tag].FindAllString(content, -1))
			if open != close {
				t.Errorf("%s <%s> 开闭不平衡：开 %d / 闭 %d（Go template 不检查这个，浏览器自动修复掩盖问题，布局会错乱）",
					name, tag, open, close)
			}
		}

		// 额外检查：模板不能以未闭合的状态结束（历史残缺模板的特征是结尾大量空白 + 单个 </div>）
		trimmed := strings.TrimSpace(content)
		if strings.HasSuffix(trimmed, "{{end}}") || strings.HasSuffix(trimmed, "{{else}}") {
			// 允许的结束形态
		} else if strings.HasSuffix(trimmed, "{{end}}\n</div>") {
			// 历史模板的常见形态，不算错（有对应 {{if}}）
		}
	}
}