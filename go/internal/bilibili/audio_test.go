package bilibili

import (
	"encoding/json"
	"net/http"
	"testing"
)

// viewInfoBody B 站 view 接口响应（GetVideoInfo）
func viewInfoBody(title string) string {
	b, _ := json.Marshal(map[string]any{
		"code": 0, "message": "success",
		"data": map[string]any{
			"bvid": "BV1xx", "aid": 11, "cid": 123, "title": title,
			"pic": "//i0.hdslb.com/v.jpg", "duration": 225,
			"owner": map[string]any{"mid": 1, "name": "某UP主", "face": "//f.jpg"},
		},
	})
	return string(b)
}

// playurlBody DASH 音频响应（B 站真实档位：30280=192k/30232=128k/30216=64k）
func playurlBody(withDash, withDurl bool) string {
	data := map[string]any{"quality": 30216, "format": "mp4"}
	if withDash {
		data["dash"] = map[string]any{
			"audio": []map[string]any{
				{"id": 30216, "baseUrl": "http://cdn.example/30216.m4s", "backupUrl": []string{},
					"bandwidth": 64000, "mimeType": "audio/mp4", "codecs": "mp4a.40.2", "size": 1000},
				{"id": 30232, "baseUrl": "http://cdn.example/30232.m4s", "backupUrl": []string{},
					"bandwidth": 128000, "mimeType": "audio/mp4", "codecs": "mp4a.40.2", "size": 2000},
				{"id": 30280, "baseUrl": "", "backupUrl": []string{"http://cdn.example/30280-backup.m4s"},
					"bandwidth": 192000, "mimeType": "audio/mp4", "codecs": "mp4a.40.2", "size": 3000},
			},
		}
	}
	if withDurl {
		data["durl"] = []map[string]any{{"url": "http://cdn.example/durl.mp4", "size": 500}}
	}
	b, _ := json.Marshal(map[string]any{"code": 0, "message": "success", "data": data})
	return string(b)
}

// TestGetVideoInfo view 接口解析
func TestGetVideoInfo(t *testing.T) {
	client, _ := newFakeBili(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/x/web-interface/view" || r.URL.Query().Get("bvid") != "BV1xx" {
			t.Errorf("请求异常: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(viewInfoBody("测试视频")))
	})
	info, err := client.GetVideoInfo("BV1xx")
	if err != nil {
		t.Fatalf("GetVideoInfo: %v", err)
	}
	if info.Title != "测试视频" || info.CID != 123 || info.Duration != 225 || info.Owner.Name != "某UP主" {
		t.Errorf("解析异常: %+v", info)
	}
}

// TestGetAudioStreamParams playurl 请求参数锁死（fnval=16 DASH、fourk=1、qn）
func TestGetAudioStreamParams(t *testing.T) {
	client, _ := newFakeBili(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/x/player/playurl" {
			t.Errorf("path=%q", r.URL.Path)
		}
		q := r.URL.Query()
		for k, want := range map[string]string{
			"bvid": "BV1xx", "cid": "123", "qn": "30280", "fnval": "16", "fourk": "1",
		} {
			if q.Get(k) != want {
				t.Errorf("query[%s]=%q, want %q", k, q.Get(k), want)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(playurlBody(true, false)))
	})
	if _, err := client.GetAudioStream("BV1xx", 123, 30280); err != nil {
		t.Fatalf("GetAudioStream: %v", err)
	}
}

// TestGetBestAudioURL_DashPrefers DASH 优先 + 按首选音质选 + baseURL 空用 backup
func TestGetBestAudioURL_DashPrefers(t *testing.T) {
	client, _ := newFakeBili(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(playurlBody(true, false)))
	})
	// 首选 128 → 30232 命中
	res, err := client.GetBestAudioURL("BV1xx", 123, 30232)
	if err != nil {
		t.Fatalf("GetBestAudioURL: %v", err)
	}
	if res.Quality != 30232 || res.URL != "http://cdn.example/30232.m4s" {
		t.Errorf("首选30232: %+v", res)
	}
	// 首选 192 → baseURL 空 → backup
	res, err = client.GetBestAudioURL("BV1xx", 123, 30280)
	if err != nil {
		t.Fatalf("GetBestAudioURL: %v", err)
	}
	if res.URL != "http://cdn.example/30280-backup.m4s" {
		t.Errorf("30280 backup: %+v", res)
	}
	// 首选一个没有的档位 → 顺位 QualityPriority（30280 首位）
	res, err = client.GetBestAudioURL("BV1xx", 123, 20248)
	if err != nil {
		t.Fatalf("GetBestAudioURL: %v", err)
	}
	if res.Quality != 30280 { // QualityPriority 首位
		t.Errorf("降档后: %+v", res)
	}
}

// TestGetBestAudioURL_DurlFallback 无 DASH 有 durl → durl 兜底
func TestGetBestAudioURL_DurlFallback(t *testing.T) {
	client, _ := newFakeBili(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(playurlBody(false, true)))
	})
	res, err := client.GetBestAudioURL("BV1xx", 123, 128)
	if err != nil {
		t.Fatalf("GetBestAudioURL: %v", err)
	}
	if res.URL != "http://cdn.example/durl.mp4" {
		t.Errorf("durl 兜底: %+v", res)
	}
}

// TestGetBestAudioURL_None 无任何音频 → 业务错误
func TestGetBestAudioURL_None(t *testing.T) {
	client, _ := newFakeBili(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":0,"message":"success","data":{"quality":0,"format":""}}`))
	})
	if _, err := client.GetBestAudioURL("BV1xx", 123, 128); err == nil {
		t.Fatal("want error（无音频流）")
	}
}