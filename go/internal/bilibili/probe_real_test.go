package bilibili

import (
	"encoding/json"
	"testing"
)

// 临时探测：真实 B 站音频参数（Bandwidth/Size/Codecs/真实时长），测 seek 偏差。测完删。
func TestProbeRealAudio(t *testing.T) {
	c := NewClient("https://api.bilibili.com", "Mozilla/5.0", "https://www.bilibili.com", 15000000000)
	bv := "BV1pjWu6dE36"
	info, err := c.GetVideoInfo(bv)
	if err != nil {
		t.Skipf("GetVideoInfo 失败（可能无网/风控）: %v", err)
	}
	t.Logf("视频: %s 时长=%ds cid=%d", info.Title, info.Duration, info.CID)

	for _, q := range []int{Audio192K, Audio128K, Audio64K} {
		stream, err := c.GetAudioStream(bv, info.CID, q)
		if err != nil {
			t.Logf("qn=%d GetAudioStream err=%v", q, err)
			continue
		}
		if stream.Dash == nil || len(stream.Dash.Audio) == 0 {
			t.Logf("qn=%d 无 Dash.Audio", q)
			continue
		}
		for _, a := range stream.Dash.Audio {
			avgKbps := 0
			if info.Duration > 0 && a.Size > 0 {
				avgKbps = a.Size * 8 / info.Duration / 1000
			}
			b, _ := json.Marshal(a)
			t.Logf("档位 id=%d codecs=%s mime=%s bandwidth=%d size=%d 平均码率≈%dkbps (对比固定128k偏差%.0f%%)\n  raw=%s",
				a.ID, a.Codecs, a.MimeType, a.Bandwidth, a.Size, avgKbps,
				float64(avgKbps-128)/128*100, string(b))
		}
	}
}
