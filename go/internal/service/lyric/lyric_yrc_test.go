package lyric

import (
	"testing"
)

// 对标 bili-music 的解析方法论升级：LRC [offset:] 全局偏移标签 + yrc/qrc/krc 增强格式

// [offset:-500]（负=整体提前 0.5s）应折进每行时间：t' = t - 0.5
func TestParseLRCFileOffset(t *testing.T) {
	input := `[offset:-500]
[00:05.00]第一句
[00:10.00]第二句`
	lines := parseLRC(input)
	if len(lines) != 2 {
		t.Fatalf("行数=%d, want 2", len(lines))
	}
	if lines[0][0].(float64) != 4.5 || lines[1][0].(float64) != 9.5 {
		t.Fatalf("offset 未折入: %+v, want [4.5, 9.5]", lines)
	}
	if lines[0][1].(string) != "第一句" {
		t.Fatalf("文本错: %+v", lines[0])
	}
	// 正偏移：整体延后
	lines2 := parseLRC("[offset:500]\n[00:05.00]第一句")
	if lines2[0][0].(float64) != 5.5 {
		t.Fatalf("正偏移折入错: %+v", lines2)
	}
	// 兼容逗号分隔 [offset,-500]
	lines3 := parseLRC("[offset,-500]\n[00:05.00]第一句")
	if lines3[0][0].(float64) != 4.5 {
		t.Fatalf("逗号分隔 offset 错: %+v", lines3)
	}
}

// LRC 行无自带时长 → l[2]=0（前端用下一行差估算进度）
func TestParseLRCDurationUnknown(t *testing.T) {
	lines := parseLRC("[00:05.00]第一句\n[00:10.00]第二句")
	if lines[0][2].(float64) != 0 {
		t.Fatalf("LRC 行时长应为 0(未知): %+v", lines[0])
	}
}

// yrc 逐字：行级 [start,dur] + 字级 (…) 剥离 + 自带行时长 + 元信息行过滤 + offset 折入
func TestParseYRC(t *testing.T) {
	input := `[0,1000](0,1000,0) 作曲 : 周杰伦
[1000,2000](1000,500,0) 第一句
[3000,2000](3000,1000,0)(4000,1000,0) 第二句歌词
(纯音乐)
[8000,1000](8000,1000,0) 末句`
	lines := parseYRC(input)
	if len(lines) != 3 {
		t.Fatalf("yrc 行数=%d, want 3（过滤元信息行/无词行）; %+v", len(lines), lines)
	}
	if lines[0][0].(float64) != 1.0 || lines[0][1].(string) != "第一句" || lines[0][2].(float64) != 2.0 {
		t.Fatalf("第一句错: %+v, want [1.0 第一句 2.0]", lines[0])
	}
	if lines[1][1].(string) != "第二句歌词" { // 字级时间戳应被剥离
		t.Fatalf("字级时间戳未剥离: %+v", lines[1])
	}
	if lines[1][2].(float64) != 2.0 || lines[2][0].(float64) != 8.0 {
		t.Fatalf("时长/时间错: %+v", lines[1:])
	}
	// offset 标签（krc 风格）折入
	lines2 := parseYRC("[offset,-1000]\n[2000,1000](2000,1000,0) 第一句")
	if lines2[0][0].(float64) != 1.0 {
		t.Fatalf("yrc offset 折入错: %+v", lines2)
	}
}

// parseLyrics 自动检测：增强格式 → yrc；标准 LRC → lrc
func TestParseLyricsAutoDetect(t *testing.T) {
	yrc := "[1000,2000](1000,500,0) 第一句\n[3000,2000](3000,1000,0) 第二句"
	if lines := parseLyrics(yrc); lines == nil || lines[0][2].(float64) != 2.0 {
		t.Fatalf("增强格式应走 yrc 解析（带行时长）: %+v", lines)
	}
	lrc := "[00:05.00]第一句\n[00:10.00]第二句"
	if lines := parseLyrics(lrc); lines == nil || lines[0][2].(float64) != 0 {
		t.Fatalf("标准 LRC 应走 lrc 解析（时长未知）: %+v", lines)
	}
	// 混合元信息头（ti: 标签行）不影响检测
	mixed := "[ti:test]\n[offset:0]\n[00:05.00]第一句"
	if lines := parseLyrics(mixed); lines == nil || len(lines) != 1 {
		t.Fatalf("带元信息头的 LRC 解析错: %+v", lines)
	}
}
