package widget

import (
	"encoding/json"
	"runtime"
	"sync"
	"testing"
	"time"
)

// ── 全 mock：不联网、不依赖真实 lyric service、不起进程 ──

// fakeSource 测试替身（实现 LyricSource，返回固定行 + offset）
type fakeSource struct {
	mu     sync.Mutex
	lines  [][3]any
	offset float64
	ok     bool
	calls  int
}

func (f *fakeSource) Lines(bvid string) ([][3]any, float64, bool) {
	f.mu.Lock()
	f.calls++
	ok, lines, off := f.ok, f.lines, f.offset
	f.mu.Unlock()
	if !ok || bvid == "" {
		return nil, 0, false
	}
	return lines, off, true
}

func TestPickLine_位置到行定位(t *testing.T) {
	lines := [][3]any{
		{0.0, "前奏", 0.0},
		{10.0, "天空海阔", 0.0},
		{25.0, "任意东西", 0.0},
		{40.0, "最后一段", 0.0},
	}
	cases := []struct {
		t    float64
		want string
	}{
		{5, "前奏"},
		{10, "天空海阔"}, // sec<=t 边界
		{12, "天空海阔"},
		{24, "天空海阔"},
		{25, "任意东西"},
		{50, "最后一段"},
		{-1, ""}, // 负时间无行
	}
	for i, c := range cases {
		if got := PickLine(lines, c.t); got != c.want {
			t.Fatalf("case%d t=%.1f: got %q want %q", i, c.t, got, c.want)
		}
	}
}

func TestPickLine_空(t *testing.T) {
	if got := PickLine(nil, 5); got != "" {
		t.Fatalf("empty: got %q want empty", got)
	}
}

func TestCurrentLine_缓存与offset(t *testing.T) {
	src := &fakeSource{
		lines:  [][3]any{{0, "A", 0}, {20, "B", 0}},
		offset: 3, ok: true,
	}
	w := NewWidgetState()
	w.SetLineSrc(src)

	// 先 currentLine（触发异步拉）→ 等拉完 → 断言
	_ = w.currentLine("x", 15) // 触发 ensureLines
	waitCalls(t, src, 1)
	if got := w.currentLine("x", 15); got != "A" {
		t.Fatalf("ct15: got %q want A", got)
	}
	// ct=25+offset3=28 → sec<=28 → B（同 bvid 命中缓存，不再调源）
	if got := w.currentLine("x", 25); got != "B" {
		t.Fatalf("ct25: got %q want B", got)
	}
	if src.calls != 1 {
		t.Fatalf("同 bvid 应只拉一次源，calls=%d", src.calls)
	}
}

// waitCalls 轮询等 fakeSource 被调用 n 次（异步拉 goroutine 完成，不固定 sleep）
func waitCalls(t *testing.T, src *fakeSource, n int) {
	t.Helper()
	for i := 0; i < 2000; i++ {
		src.mu.Lock()
		got := src.calls
		src.mu.Unlock()
		if got >= n {
			return
		}
		runtime.Gosched()
	}
	src.mu.Lock()
	got := src.calls
	src.mu.Unlock()
	t.Fatalf("等拉源超时：calls=%d want %d", got, n)
}

// waitLine 轮询等 w 的当前行缓存就绪（异步拉完成后），再断言
func waitLine(t *testing.T, w *WidgetState, bvid string, ct float64) string {
	t.Helper()
	for i := 0; i < 2000; i++ {
		w.mu.Lock()
		ok := w.cacheOK && w.cacheBvid == bvid
		w.mu.Unlock()
		if ok {
			return w.currentLine(bvid, ct)
		}
		runtime.Gosched()
	}
	return w.currentLine(bvid, ct)
}

func TestCurrentLine_无源或空(t *testing.T) {
	w := NewWidgetState()
	if got := w.currentLine("x", 5); got != "" {
		t.Fatalf("无源: got %q want empty", got)
	}
	w.SetLineSrc(&fakeSource{ok: false})
	if got := w.currentLine("x", 5); got != "" {
		t.Fatalf("源不 ok: got %q want empty", got)
	}
}

func TestBroadcastLyricState_带Line(t *testing.T) {
	src := &fakeSource{
		lines:  [][3]any{{0, "前奏", 0}, {10, "天空海阔", 0}, {25, "任意东西", 0}},
		offset: 0, ok: true,
	}
	w := NewWidgetState()
	w.SetLineSrc(src)

	// mock 订阅者（chan 缓冲，读第一条 lyric-state）
	ch := w.Subscribe()
	defer w.Unsubscribe(ch)

	// 触发广播（ct=12 → 行"天空海阔"）；首次可能空（异步拉未到），等拉到再读
	w.Set("BVx", "海阔天空", "Beyond", 12, 300, true)

	// 等异步拉完（cacheOK=true），再读订阅者里最新 lyric-state
	waitLine(t, w, "BVx", 12)

	// 读订阅者直到拿到带真 line 的 lyric-state（首条可能空，异步拉到后第二条带真值）
	var got SSEMsg
	found := false
	for i := 0; i < 2000 && !found; i++ {
		select {
		case got = <-ch:
			var p map[string]any
			_ = json.Unmarshal([]byte(got.Data), &p)
			if p["line"] == "天空海阔" {
				found = true
			}
		default:
			runtime.Gosched()
		}
	}
	if !found {
		t.Fatal("没拿到带真 line 的广播")
	}
	if got.Name != "lyric-state" {
		t.Fatalf("事件名=%q want lyric-state", got.Name)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(got.Data), &payload); err != nil {
		t.Fatalf("payload 非 JSON: %v", err)
	}
	if payload["line"] != "天空海阔" {
		t.Fatalf("line=%v want 天空海阔", payload["line"])
	}
	if payload["bvid"] != "BVx" || payload["current_time"] != 12.0 {
		t.Fatalf("快照字段错: %v", payload)
	}
}

func TestBroadcastLyricState_切歌拉源(t *testing.T) {
	src := &fakeSource{
		lines:  [][3]any{{0, "行A", 0}, {20, "行B", 0}},
		offset: 0, ok: true,
	}
	w := NewWidgetState()
	w.SetLineSrc(src)
	ch := w.Subscribe()
	defer w.Unsubscribe(ch)

	// 切歌（bvid 变）→ 触发异步拉源；等拉完再断言
	w.Set("BV1", "歌1", "a", 5, 100, true)
	waitCalls(t, src, 1)
	waitLine(t, w, "BV1", 5)
	if got := w.currentLine("BV1", 5); got != "行A" {
		t.Fatalf("切歌后 line=%q want 行A", got)
	}
	// 同 bvid 再上报 → 不拉源（缓存命中）
	w.Set("BV1", "歌1", "a", 22, 100, true)
	if got := w.currentLine("BV1", 22); got != "行B" {
		t.Fatalf("同 bvid line=%q want 行B", got)
	}
	src.mu.Lock()
	calls := src.calls
	src.mu.Unlock()
	if calls != 1 {
		t.Fatalf("同 bvid 不应再拉源 calls=%d", calls)
	}
}

var _ = sync.Mutex{}

// TestEnsureLinesRetryAfterFail 拉取失败后必须能重试：
// 旧 bug 是 pendingBvid 拉完不清 → 首次失败后 cacheOK 恒 false 且每次
// currentLine 都误判"在途"直接 return → 该歌歌词永远拉不到（line 恒空，真机踩过）。
func TestEnsureLinesRetryAfterFail(t *testing.T) {
	w := NewWidgetState()
	src := &fakeSource{ok: false} // 首次失败
	w.SetLineSrc(src)
	// 首次 ensureLines：异步拉，失败
	w.currentLine2("BV1x", 0) // 触发 ensureLines（异步）
	// 等 goroutine 完成（失败不给广播，轮询 fakeSource.calls 判断结束）
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		src.mu.Lock()
		c := src.calls
		src.mu.Unlock()
		if c > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	// 切到成功源，越过失败冷却（直接清 lastFailAt，模拟 5s 后）
	src.mu.Lock()
	src.ok = true
	src.lines = [][3]any{{0.0, "第一行", 0.0}}
	src.mu.Unlock()
	w.mu.Lock()
	w.lastFailAt = time.Time{}
	w.mu.Unlock()

	// 再拉：必须真的发起第二次拉取并拿到行（旧 bug 这里 calls 不再增长 / line 恒空）
	w.currentLine2("BV1x", 0)
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if line, _ := w.currentLine2("BV1x", 0); line == "第一行" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	src.mu.Lock()
	c := src.calls
	src.mu.Unlock()
	t.Fatalf("失败后重试未生效：calls=%d（应在失败冷却后重新拉并拿到行）", c)
}
