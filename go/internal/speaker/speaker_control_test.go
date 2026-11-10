package speaker

import "testing"

// TestNeedPlayMusicAPI 硬件分文：需要 music API 的设备（LX05A/L07A 等）走 play_by_music_url。
// 对齐 MiAir/xiaomusic NEED_USE_PLAY_MUSIC_API（含用户两台设备 L07A + LX05A）。
func TestNeedPlayMusicAPI(t *testing.T) {
	cases := []struct {
		hw   string
		want bool
	}{
		{"L07A", true},   // Redmi小爱音箱Play L07A（用户设备）
		{"LX05A", true},  // 小爱音箱万能遥控版 LX05A（用户设备）
		{"LX05", true},   // 万能遥控版
		{"X08C", true},   // 小爱音箱
		{"L15A", true},   // 小爱音箱 Pro
		{"X6A", true},
		{"OH2", true},
		{"OH2P", true},
		{"X4B", true},
		{"X8F", true},
		{"X08E", true},
		// 不需要 music API 的设备（非名单）
		{"", false},
		{"unknown", false},
		{"LX01", false},
		{"L06", false},
	}
	for _, c := range cases {
		if got := NeedPlayMusicAPI(c.hw); got != c.want {
			t.Errorf("NeedPlayMusicAPI(%q)=%v want %v", c.hw, got, c.want)
		}
	}
}

// TestPlayURLBranch 验证 PlayURL 按硬件分文（PlayURL 无网络时 client=nil 返回 nil，仅测分支不崩）
func TestPlayURLBranch(t *testing.T) {
	a := &SpeakerAuth{client: nil}
	// client=nil → 返回 nil,nil（不崩），分支代码路径可达
	a.Hardware = "L07A"
	if _, err := a.PlayURL("did", "http://x"); err != nil {
		t.Fatalf("PlayURL(nil client) should not err: %v", err)
	}
	a.Hardware = "unknown"
	if _, err := a.PlayURL("did", "http://x"); err != nil {
		t.Fatalf("PlayURL unknown hw should not err: %v", err)
	}
}
