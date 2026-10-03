package bilibili

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// newFakeBili 假 B 站服务器：验证请求构造 + 响应解析（真 HTTP，非 mock 库）
// handler 可自定义；默认回 code=0 + 一条带 <em> 高亮的搜索项
func newFakeBili(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, "TestUA/1.0", "https://ref.example.com", 5*time.Second), srv
}

// biliSearchBody 生成 B 站风格搜索响应
func biliSearchBody(highlight bool) string {
	title := "周杰伦《瓦解》现场"
	if highlight {
		title = `<em class="keyword">周杰伦</em>《瓦解》现场`
	}
	resp := map[string]any{
		"code":    0,
		"message": "success",
		"data": map[string]any{
			"numResults": 1,
			"result": []map[string]any{
				{"id": 1, "bvid": "BV1xx", "title": title, "duration": "3:45",
					"pic": "//i0.hdslb.com/1.jpg", "author": "某UP主", "typename": "音乐"},
			},
		},
	}
	b, _ := json.Marshal(resp)
	return string(b)
}

// TestSearchRequestsCorrect 请求构造锁死（对齐 python 版细节：duration=1 短时、tids=3 音乐区）
func TestSearchRequestsCorrect(t *testing.T) {
	var gotPath, gotQuery string
	client, _ := newFakeBili(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		// UA / Referer 必须带上（B站防风控关键，对齐 python）
		if ua := r.Header.Get("User-Agent"); !strings.Contains(ua, "TestUA") {
			t.Errorf("UA=%q, want 含 TestUA", ua)
		}
		if ref := r.Header.Get("Referer"); ref != "https://ref.example.com" {
			t.Errorf("Referer=%q", ref)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(biliSearchBody(false)))
	})

	if _, err := client.Search("周杰伦", 2, 30); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if gotPath != "/x/web-interface/search/type" {
		t.Errorf("path=%q", gotPath)
	}
	q := parseQuery(t, gotQuery)
	for k, want := range map[string]string{
		"keyword": "周杰伦", "search_type": "video", "page": "2", "pagesize": "30",
		"duration": "1", "tids": "3", // 对齐 python：只搜 10 分钟以下音乐区
	} {
		if q.Get(k) != want {
			t.Errorf("query %s=%q, want %q", k, q.Get(k), want)
		}
	}
}

// TestSearchParsesAndCleans 解析 + 清洗 <em> 高亮标签（B站搜索标题的真实形态）
func TestSearchParsesAndCleans(t *testing.T) {
	client, _ := newFakeBili(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(biliSearchBody(true)))
	})

	res, err := client.Search("周杰伦", 1, 30)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if res.NumResults != 1 || len(res.Result) != 1 {
		t.Fatalf("结果 = %+v", res)
	}
	it := res.Result[0]
	if it.Title != "周杰伦《瓦解》现场" { // <em> 已剥
		t.Errorf("Title=%q, want 清洗后纯文本", it.Title)
	}
	if it.BVID != "BV1xx" || it.Duration != "3:45" || it.Author != "某UP主" {
		t.Errorf("字段解析异常: %+v", it)
	}
}

// TestSearchAPIBusinessError B站业务错误（code != 0）→ BilibiliAPIError
func TestSearchAPIBusinessError(t *testing.T) {
	client, _ := newFakeBili(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":-400,"message":"请求错误","data":null}`))
	})
	_, err := client.Search("x", 1, 10)
	apiErr, ok := err.(*BilibiliAPIError)
	if !ok {
		t.Fatalf("err=%v (%T), want *BilibiliAPIError", err, err)
	}
	if apiErr.Code != -400 || apiErr.Message != "请求错误" {
		t.Errorf("apiErr=%+v", apiErr)
	}
}

// TestSearchHTTPError 服务器 500 → 非 BilibiliAPIError 的 HTTP 错误
func TestSearchHTTPError(t *testing.T) {
	client, _ := newFakeBili(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	if _, err := client.Search("x", 1, 10); err == nil {
		t.Fatal("want error on 500")
	}
}

// TestSearchEmptyResult code=0 但空结果 → 空列表不报错
func TestSearchEmptyResult(t *testing.T) {
	client, _ := newFakeBili(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":0,"message":"success","data":{"numResults":0,"result":[]}}`))
	})
	res, err := client.Search("不存在", 1, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Result) != 0 {
		t.Errorf("结果非空: %+v", res.Result)
	}
}

// TestCleanHTML 纯函数：剥各种标签
func TestCleanHTML(t *testing.T) {
	cases := map[string]string{
		"<em class=\"keyword\">周杰伦</em>《瓦解》": "周杰伦《瓦解》",
		"<b>加粗</b>标题":                           "加粗标题",
		"无标签纯文本":                               "无标签纯文本",
		"": "",
	}
	for in, want := range cases {
		if got := CleanHTML(in); got != want {
			t.Errorf("CleanHTML(%q)=%q, want %q", in, got, want)
		}
	}
}

// parseQuery 解析 URL query（测试工具）
func parseQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	q, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("ParseQuery(%q): %v", raw, err)
	}
	return q
}