package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"homecast/internal/bilibili"
)

// TestParseDurationSec 时长解析纯函数：m:ss / h:mm:ss / 纯秒 / 非法 → 0
func TestParseDurationSec(t *testing.T) {
	cases := map[string]int{
		"3:45": 225, "1:02:03": 3723, "0:30": 30, "45": 45,
		"": 0, "abc": 0, "12:3:4": 12*3600 + 3*60 + 4, "1:": 1, "：3": 0,
	}
	for in, want := range cases {
		if got := ParseDurationSec(in); got != want {
			t.Errorf("ParseDurationSec(%q)=%d, want %d", in, got, want)
		}
	}
}

// TestSearchRank 排序权重纯函数（对齐 python _rank：符号越少越靠前）
func TestSearchRank(t *testing.T) {
	cases := []struct {
		title string
		typ   string
		sec   int
		want  int
	}{
		{"周杰伦《瓦解》", "音乐", 200, 0},    // 《》不在计数符号表([（【]等) → 0
		{"告白气球", "音乐", 200, 0},                     // 干净标题
		{"【4K】周杰伦现场", "音乐", 200, 4},               // 【】2 + 4k + 现场 包装词 = 4
		{"零基础学吉他【教程】", "音乐教学", 120, 4}, // 【】2符号 + 教学+2
		{"周杰伦演唱会完整版", "音乐", 30, 2},               // 过短 +2
		{"周杰伦演唱会完整版", "音乐", 700, 2},               // 过长 +2
	}
	for _, c := range cases {
		got := searchRank(MusicItem{Title: c.title, Typename: c.typ, DurationSec: c.sec})
		if got != c.want {
			t.Errorf("searchRank(%q) = %d, want %d", c.title, got, c.want)
		}
	}
}

// fakeBili 假 B 站服务器：可喂多次响应（按请求路径区分）
func fakeBili(t *testing.T, fn http.HandlerFunc) *bilibili.Client {
	t.Helper()
	srv := httptest.NewServer(fn)
	t.Cleanup(srv.Close)
	return bilibili.NewClient(srv.URL, "TestUA/1.0", "https://ref.example.com", 5*time.Second)
}

// searchBodyWith 生成指定搜索结果的响应
func searchBodyWith(items []map[string]any) string {
	b, _ := json.Marshal(map[string]any{
		"code": 0, "message": "success",
		"data": map[string]any{"numResults": len(items), "result": items},
	})
	return string(b)
}

// TestMusicServiceSearch 搜索排序：权重升序 + 同权重时长升序 + 无时长滤掉
func TestMusicServiceSearch(t *testing.T) {
	client := fakeBili(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(searchBodyWith([]map[string]any{
			{"bvid": "BV-A", "title": "【4K】周杰伦《最伟大的作品》", "duration": "4:00", "author": "甲", "typename": "音乐", "play": 10},
			{"bvid": "BV-B", "title": "告白气球", "duration": "3:00", "author": "乙", "typename": "音乐", "play": 99},
			{"bvid": "BV-C", "title": "无时长滤掉", "duration": "", "author": "丙", "typename": "音乐", "play": 1},
			{"bvid": "BV-D", "title": "晴天", "duration": "2:30", "author": "丁", "typename": "音乐", "play": 50},
			{"bvid": "BV-E", "title": "零基础教程", "duration": "5:00", "author": "戊", "typename": "音乐教学", "play": 5},
		})))
	})
	m := &MusicService{Client: client}
	res, err := m.Search("周杰伦", 1, 30)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// 干净标题"晴天"(0) 与 "告白气球"(0) 排序：同权重时长升序 → 告白气球150s 晴天150s？
	// 2:30=150s 3:00=180s → 晴天(150) 在 告白气球(180) 前
	got := make([]string, len(res.List))
	for i, it := range res.List {
		got[i] = it.BVID
	}
	// 期望排序（rank 升序，同 rank 时长升序）：
	// BV-D 晴天 rank0/150s ← BV-B 告白气球 rank0/180s（4:00=240 晴天150 在先）
	//   —— 等等：BV-B 3:00=180 > 晴天150 → 晴天先
	// BV-A 【4K】… rank3（符号+包装词）/240s ← BV-E 教学 rank5？ 
	//   教学 = 符号0 + 教学+2 = 2？【教程】没有——'零基础教程' 无括号 → 教学+2 → rank2
	// 重新算：BV-A: 【4K】=【】2符号 + 4k包装词 = 3；BV-E: 音乐教学+2 = 2
	// 排序：BV-D(0)/BV-B(0) → 晴天(150) 告白气球(180)
	//      → BV-E(2) → BV-A(3)
	// 滤掉 BV-C（无时长）
	want := []string{"BV-D", "BV-B", "BV-E", "BV-A"}
	if len(got) != len(want) {
		t.Fatalf("结果=%v, want %v（BV-C 无时长应被滤掉）", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("顺序[%d]=%s, want %s; got=%v", i, got[i], want[i], got)
		}
	}
}