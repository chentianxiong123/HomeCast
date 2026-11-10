package speaker

import "testing"

// TestNeedPlayMusicAPI 硬件分文：需要 music API 的设备走 play_by_music_url。
// 对齐 xiaomusic NEED_USE_PLAY_MUSIC_API（不含 LX05A/L07A/L15A）。
// 实测反转：LX05A 用 player_play_url 成功 / play_by_music_url 失败，
// 所以 LX05A/L07A/L15A 不在名单（MiAir 含它们是错的）。
func TestNeedPlayMusicAPI(t *testing.T) {
	cases := []struct {
		hw   string
		want bool
	}{
		// xiaomusic 名单内（走 music API）
		{"LX05", true}, // 万能遥控版（注意：LX05 在，LX05A 不在）
		{"X08C", true}, // 小爱音箱
		{"X08E", true},
		{"X8F", true},
		{"X4B", true},
		{"X6A", true},
		{"OH2", true},
		{"OH2P", true},
		// xiaomusic 名单外（走 play_by_url，实测 LX05A 用 play_by_url 成功）
		{"LX05A", false}, // 小爱音箱万能遥控版 LX05A（用户设备，实测 play_by_url code:0）
		{"L07A", false},  // Redmi小爱音箱Play L07A（用户设备）
		{"L15A", false},  // 小爱音箱 Pro
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
