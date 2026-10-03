package service

import (
	"testing"
)

// TestParseLRC 纯函数：LRC 歌词文本解析（无网络，逻辑锁定）
// 覆盖：标准时间标签、乱序、多时间戳、空白行、纯器乐占位、非歌词行、毫秒精度
func TestParseLRC(t *testing.T) {
	input := `[ti:test]
[00:05.00]第一句
[00:10.50]第二句
[00:07.00]乱序的
[00:12.00]
[01:00.123]毫秒精度
(纯音乐，请欣赏)
[02:00.00]末句
[00:03.00]前奏（器乐）`
	lines := parseLRC(input)
	if len(lines) != 6 {
		t.Fatalf("parseLRC 行数=%d, want 6（跳过空行/占位/非歌词行）; lines=%+v", len(lines), lines)
	}
	// 乱序应被排序（按时间）
	for i := 1; i < len(lines); i++ {
		if lines[i][0].(float64) < lines[i-1][0].(float64) {
			t.Errorf("未按时间排序: %+v", lines)
		}
	}
	if lines[0][1].(string) != "前奏（器乐）" { // 00:03 最先
		t.Errorf("lines[0]=%+v, want 前奏（器乐）", lines[0])
	}
	last := lines[len(lines)-1]
	if last[0].(float64) != 120 || last[1].(string) != "末句" {
		t.Errorf("末句解析异常: %+v", last)
	}
	// 毫秒精度：[01:00.123] → 60.123（在 120 之前）
	ms := lines[4]
	if ms[0].(float64) != 60.123 || ms[1].(string) != "毫秒精度" {
		t.Errorf("毫秒精度解析异常: %+v", ms)
	}
}

// TestParseLRCEmpty 无有效歌词行 → nil
func TestParseLRCEmpty(t *testing.T) {
	for _, in := range []string{"", "\n\n", "[00:01.00]\n(纯音乐)", "纯文本没有时间标签"} {
		if got := parseLRC(in); got != nil {
			t.Errorf("parseLRC(%q)=%+v, want nil", in, got)
		}
	}
}