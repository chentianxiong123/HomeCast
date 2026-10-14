package speaker

import (
	"encoding/json"
	"fmt"
	"testing"
)

// 回归：小米接口 userId 是 JSON 数字（如 2908798005），
// 旧版 fmt.Sprint(float64) 会输出科学计数法 "2.908798005e+09" 导致登录必败。
// 修复：UseNumber 解码 + fixSciNotation 清洗历史坏值。
func TestDecodeJSONNumUserId(t *testing.T) {
	// 模拟小米长轮询响应的 userId 数字字段
	resp := `{"code":0,"userId":2908798005,"passToken":"abc"}`
	var m map[string]any
	if err := json.Unmarshal([]byte(resp), &m); err != nil {
		t.Fatal(err)
	}
	// 旧行为（float64 + fmt.Sprint）→ 科学计数法坏值
	if old := fmt.Sprint(m["userId"]); old != "2.908798005e+09" {
		t.Logf("注意: 当前环境旧式输出=%q（若已是 2908798005 说明 json 行为不同）", old)
	}
	// 新行为（decodeJSONNum + fmt.Sprint）→ 原始字符串
	var m2 map[string]any
	if err := decodeJSONNum(resp, &m2); err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprint(m2["userId"])
	if got != "2908798005" {
		t.Fatalf("decodeJSONNum userId=%q, want 2908798005（科学计数法 bug 回归）", got)
	}
}

// fixSciNotation 历史坏值清洗：2.908798005e+09 → 2908798005
func TestFixSciNotation(t *testing.T) {
	cases := []struct{ in, want string; bad bool }{
		{"2.908798005e+09", "2908798005", true},
		{"2908798005", "2908798005", false},
		{"", "", false},
		{"abc", "abc", false},
	}
	for _, c := range cases {
		got, bad := fixSciNotation(c.in)
		if bad != c.bad || got != c.want {
			t.Errorf("fixSciNotation(%q) = (%q,%v), want (%q,%v)", c.in, got, bad, c.want, c.bad)
		}
	}
}

// 完整的扫码成功提取逻辑：authData[key] = fmt.Sprint(v)（v 是 json.Number）
func TestQRFieldExtraction(t *testing.T) {
	resp := `{"code":0,"userId":2908798005,"passToken":"tok","cUserId":"cuid","serviceToken":"stok","ssecurity":"ssec","nonce":"n","psecurity":"p"}`
	var lpData map[string]any
	if err := decodeJSONNum(resp, &lpData); err != nil {
		t.Fatal(err)
	}
	keys := []string{"psecurity", "nonce", "ssecurity", "passToken", "userId", "cUserId", "serviceToken"}
	q := &QRLogin{authData: map[string]string{}}
	for _, k := range keys {
		if v, ok := lpData[k]; ok {
			q.authData[k] = fmt.Sprint(v)
		}
	}
	if q.authData["userId"] != "2908798005" {
		t.Fatalf("authData userId=%q, want 2908798005", q.authData["userId"])
	}
	if q.authData["passToken"] != "tok" {
		t.Fatalf("passToken=%q", q.authData["passToken"])
	}
	if !numEq0(lpData["code"]) {
		t.Fatal("numEq0(code) 应为 true")
	}
}