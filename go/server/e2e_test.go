package server

// E2E 后端全链路：启动真实 server.New()（假 B站 + 临时 DB，环境变量注入），
// 走真实 HTTP（httptest.Server 包装），断言核心端点 + SSE 推送。
// 不依赖外网（B站走假 server）、不污染用户数据（临时 DB）。

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newE2E 起假 B站 + 临时 DB + 真实 server handler，返回 httptest server
func newE2E(t *testing.T, biliHandler http.HandlerFunc) *httptest.Server {
	t.Helper()
	bili := httptest.NewServer(biliHandler)
	t.Cleanup(bili.Close)
	t.Setenv("HC_BILI_BASE", bili.URL)
	t.Setenv("HC_DATA_DIR", t.TempDir())
	srv := httptest.NewServer(New())
	t.Cleanup(func() { srv.CloseClientConnections(); srv.Close() })
	return srv
}

// getJSON GET + 解信封 {code,message,data}
func getJSON(t *testing.T, url string, out any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET %s status=%d body=%s", url, resp.StatusCode, body)
	}
	var env struct {
		Code int             `json:"code"`
		Msg  string          `json:"message"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode %s: %v", url, err)
	}
	if env.Code != 0 {
		t.Fatalf("GET %s code=%d msg=%s", url, env.Code, env.Msg)
	}
	if out != nil && len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, out); err != nil {
			t.Fatalf("unmarshal data %s: %v", url, err)
		}
	}
}

// postJSON POST JSON + 解信封
func postJSON(t *testing.T, url string, body any) {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		rb, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST %s status=%d body=%s", url, resp.StatusCode, rb)
	}
	var env struct {
		Code int `json:"code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode %s: %v", url, err)
	}
	if env.Code != 0 {
		t.Fatalf("POST %s code=%d", url, env.Code)
	}
}

// TestE2EHome 首页渲染：真实 HTTP 拿 htmx 页面，断言关键 DOM 骨架
func TestE2EHome(t *testing.T) {
	srv := newE2E(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unexpected: "+r.URL.Path, 500)
	})
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	for _, want := range []string{"<meta charset=\"UTF-8\">", "player-dock", "search", "PingFang SC"} {
		if !strings.Contains(s, want) {
			t.Errorf("首页缺少 %q", want)
		}
	}
}

// TestE2EFavCRUD 收藏增删闭环：add → list（+1）→ remove → list（-1），真 DB
func TestE2EFavCRUD(t *testing.T) {
	srv := newE2E(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unexpected", 500)
	})
	var list []map[string]any
	getJSON(t, srv.URL+"/api/v1/fav/list", &list)
	before := len(list)

	postJSON(t, srv.URL+"/api/v1/fav/add", map[string]string{
		"bvid": "BV1xx411c7mD", "title": "E2E 测试歌", "artist": "测试歌手", "cover": "http://x/cover.jpg",
	})
	getJSON(t, srv.URL+"/api/v1/fav/list", &list)
	if len(list) != before+1 {
		t.Fatalf("add 后 list=%d, want %d", len(list), before+1)
	}

	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/fav/BV1xx411c7mD", nil)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE fav status=%d", resp.StatusCode)
	}
	resp.Body.Close()
	getJSON(t, srv.URL+"/api/v1/fav/list", &list)
	if len(list) != before {
		t.Fatalf("remove 后 list=%d, want %d", len(list), before)
	}
}

// TestE2EWidgetSSE 挂件 SSE：widget/state 设值 → widget/events 收到 lyric-state 推送
func TestE2EWidgetSSE(t *testing.T) {
	srv := newE2E(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unexpected", 500)
	})
	// 订阅 SSE（并发读，避免阻塞）
	events := make(chan string, 5)
	go func() {
		resp, err := http.Get(srv.URL + "/api/v1/widget/events")
		if err != nil {
			return
		}
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(line, "event: ") {
				events <- strings.TrimPrefix(line, "event: ")
			}
		}
	}()
	time.Sleep(300 * time.Millisecond) // 等订阅建立

	// 设播放状态 → 应广播 lyric-state
	postJSON(t, srv.URL+"/api/v1/widget/state", map[string]any{
		"bvid": "BV1xx411c7mD", "title": "E2E 歌", "artist": "歌手",
		"current_time": 12.5, "duration": 200.0, "playing": true,
	})
	select {
	case ev := <-events:
		if ev != "lyric-state" {
			t.Fatalf("收到事件 %q, want lyric-state", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SSE 5s 内未收到 lyric-state")
	}
}

// TestE2ELyricOffset 歌词偏移：set offset → 读回一致（真 DB 持久化）
func TestE2ELyricOffset(t *testing.T) {
	srv := newE2E(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unexpected", 500)
	})
	bvid := "BV1xx411c7mD"
	// 与前端一致：offset 走 query（FormValue 优先），body 也支持但 offset 须数字
	postJSON(t, srv.URL+"/api/v1/lyric/offset?bvid="+bvid+"&offset=2500", map[string]string{})
	var meta map[string]any
	getJSON(t, srv.URL+"/api/v1/lyric/meta?bvid="+bvid, &meta)
	if meta["offset"] != float64(2500) {
		t.Fatalf("offset=%v, want 2500", meta["offset"])
	}
}

// TestE2ESettings 主题持久化走 localStorage（前端侧），后端侧 settings 无独立 API；
// 这里验证 /hx/settings 页面可渲染（htmx 片段）。
func TestE2ESettingsPage(t *testing.T) {
	srv := newE2E(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unexpected", 500)
	})
	resp, err := http.Get(srv.URL + "/hx/settings")
	if err != nil {
		t.Fatalf("GET /hx/settings: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("settings status=%d", resp.StatusCode)
	}
}

// TestE2ESpeakerEvents 音箱 SSE 端点可订阅（返回 200 + event-stream，不依赖外网音箱）
func TestE2ESpeakerEvents(t *testing.T) {
	srv := newE2E(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unexpected", 500)
	})
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/speaker/events", nil)
	done := make(chan struct{})
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "event-stream") {
			t.Errorf("speaker events Content-Type=%q, want event-stream", ct)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("speaker events 5s 内未返回")
	}
}
