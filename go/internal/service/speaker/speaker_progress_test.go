package speaker

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"homecast/internal/bilibili"
	"homecast/internal/service/token"
	"homecast/internal/speaker"
)

// viewBody B 站 view 响应（含 title/duration）
func viewBody(title string, duration int) string {
	b, _ := json.Marshal(map[string]any{
		"code": 0, "message": "success",
		"data": map[string]any{
			"bvid": "BV1test", "title": title, "duration": duration,
			"pages": []map[string]any{{"cid": 12345, "part": "第一P", "duration": duration}},
			"cid":   12345,
		},
	})
	return string(b)
}

// playurlBody B 站 playurl 响应（DASH 音频）
func playurlBody() string {
	b, _ := json.Marshal(map[string]any{
		"code": 0, "message": "success",
		"data": map[string]any{
			"dash": map[string]any{
				"audio": []map[string]any{{
					"id": 30280, "baseUrl": "http://up.example.com/audio.m4s",
					"mimeType": "audio/mp4", "codecs": "mp4a.40.2",
				}},
			},
		},
	})
	return string(b)
}

// newTestSpeaker 音箱服务：mock B 站 + 预置设备（无 Mina 客户端 → PlayURL 返回 nil,nil）
func newTestSpeaker(t *testing.T, withClient bool) *SpeakerService {
	t.Helper()
	var client *bilibili.Client
	if withClient {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/x/web-interface/view":
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(viewBody("测试歌", 260)))
			case "/x/player/playurl":
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(playurlBody()))
			default:
				http.NotFound(w, r)
			}
		}))
		t.Cleanup(srv.Close)
		client = bilibili.NewClient(srv.URL, "TestUA/1.0", "https://ref.example.com", 5*time.Second)
	}
	s := NewSpeakerService(&speaker.SpeakerAuth{}, client, token.NewTokenStore())
	s.mu.Lock()
	s.devices["did1"] = speaker.SpeakerDevice{DID: "did1", DeviceID: "dev-1", Name: "测试音箱", Hardware: "lx05a"}
	s.mu.Unlock()
	return s
}

// TestSpeakerLocalProgress 本地模拟进度：播放中随本地时钟走
func TestSpeakerLocalProgress(t *testing.T) {
	s := newTestSpeaker(t, false)
	s.startProgress("did1", "BV1test", PlayOpts{Title: "测试歌", Duration: 260}, 30280)
	time.Sleep(150 * time.Millisecond)

	p := s.Progress("did1")
	if p["state"] != "playing" || p["bvid"] != "BV1test" {
		t.Errorf("进度状态错误: %v", p)
	}
	pos := p["position"].(float64)
	if pos <= 0 || pos > 2 {
		t.Errorf("position 应随本地时钟增长: %v", pos)
	}
	if p["duration"].(float64) != 260 {
		t.Errorf("duration = %v, want 260", p["duration"])
	}
	if p["title"] != "测试歌" {
		t.Errorf("title = %v", p["title"])
	}
}

// TestSpeakerPauseResume 暂停固定、恢复补偿（暂停时间不计入进度）
func TestSpeakerPauseResume(t *testing.T) {
	s := newTestSpeaker(t, false)
	s.startProgress("did1", "BV1test", PlayOpts{Duration: 260}, 30280)
	time.Sleep(120 * time.Millisecond)

	s.pauseProgress("did1")
	p := s.Progress("did1")
	if p["state"] != "paused" {
		t.Fatalf("暂停后 state = %v", p["state"])
	}
	frozen := p["position"].(float64)
	time.Sleep(150 * time.Millisecond)
	if again := s.Progress("did1")["position"].(float64); again != frozen {
		t.Errorf("暂停中 position 应固定: %v → %v", frozen, again)
	}

	s.resumeProgress("did1")
	time.Sleep(120 * time.Millisecond)
	pos := s.Progress("did1")["position"].(float64)
	// 恢复后继续增长；暂停的 150ms 不应计入
	if pos <= frozen || pos > frozen+0.5 {
		t.Errorf("恢复后 position 继续走且排除暂停: %v (frozen=%v)", pos, frozen)
	}
}

// TestSpeakerAutoNext 自动连播：duration 定时器到点推下一首（mock B 站取流）
func TestSpeakerAutoNext(t *testing.T) {
	s := newTestSpeaker(t, true)
	s.startProgress("did1", "BV1old", PlayOpts{Duration: 0.15, NextBVID: "BV1next"}, 30280)
	// 等定时器触发（playNextAuto → Play → startProgress 新进度）
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		p := s.Progress("did1")
		if b, _ := p["bvid"].(string); b == "BV1next" {
			return // 自动连播生效
		}
	}
	t.Fatal("自动连播未触发（3s 内 progress.bvid 未变成 BV1next）")
}

// TestSpeakerStopClears 停止后进度 idle
func TestSpeakerStopClears(t *testing.T) {
	s := newTestSpeaker(t, false)
	s.startProgress("did1", "BV1test", PlayOpts{Duration: 260, NextBVID: "BV1next"}, 30280)
	s.mu.Lock()
	s.stopProgressLocked("did1")
	s.mu.Unlock()
	p := s.Progress("did1")
	if p["state"] != "idle" || p["bvid"] != "" {
		t.Errorf("停止后应 idle: %v", p)
	}
}