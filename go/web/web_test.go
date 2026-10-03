package web

import "testing"

// TestShortTitle 标题清洗规则：只提取括号内容（《》（）【】「」及半角[]），' · ' 拼接
func TestShortTitle(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"周杰伦《瓦解》现场版", "瓦解"},
		{"【4K修复】周杰伦《最伟大的作品》", "4K修复 · 最伟大的作品"},
		{"（Live）《晴天》演唱会", "Live · 晴天"},        // 全角括号
		{"「稻香」MV", "稻香"},                          // 直角引号
		{"[1080P]《七里香》", "1080P · 七里香"},          // 半角方括号
		{"多重《A》中（B）内【C】尾「D」及[E]", "A · B · C · D · E"},
		{"七里香正常标题", "七里香正常标题"}, // 无括号 → 原题
		{"", ""},
		{"《》", "《》"}, // 空括号 → 原题
		{"(Live)英文半角圆括号", "(Live)英文半角圆括号"}, // 半角圆括号不剥
		{"   《  空白  》  ", "空白"},                  // 括号内空白修剪
	}
	for _, c := range cases {
		if got := ShortTitle(c.in); got != c.want {
			t.Errorf("shortTitle(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestTemplates 全部模板可解析（页面 + 片段），模板名 = 文件名
func TestTemplates(t *testing.T) {
	tpl, err := Templates()
	if err != nil {
		t.Fatalf("Templates() error: %v", err)
	}
	names := tpl.Templates()
	if len(names) < 15 {
		t.Errorf("模板数量过少: %d, want >= 15", len(names))
	}
	// 关键页面模板必须存在（content_music/html 已随重构淘汰，首页即搜索页）
	for _, want := range []string{"shell.html", "content_search.html",
		"content_favs.html", "content_queue.html", "content_recent.html", "content_settings.html",
		"results.html", "fav_list.html"} {
		if tpl.Lookup(want) == nil {
			t.Errorf("模板缺失: %s", want)
		}
	}
}