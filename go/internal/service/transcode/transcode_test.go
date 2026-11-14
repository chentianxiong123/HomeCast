package transcode

import "testing"

// TestSeekOffset 验证 320k CBR 续播的"秒→字节"线性换算 + 帧边界对齐。
// 纯逻辑测试（非真机）；CBR 恒定码率下换算是确定的。
func TestSeekOffset(t *testing.T) {
	tr := &Transcoder{} // SeekOffset 不依赖 db/dir，零值即可

	// startSec<=0 → 从头
	if got := tr.SeekOffset(0); got != 0 {
		t.Errorf("SeekOffset(0)=%d, want 0", got)
	}
	if got := tr.SeekOffset(-5); got != 0 {
		t.Errorf("SeekOffset(-5)=%d, want 0", got)
	}

	// 线性换算：字节 = 秒 × 40000（320000/8），向下对齐帧边界（960B）
	// 10s → 400000 字节；400000 % 960 = 400000 - 416*960 = 400000-399360=640 → 对齐到 399360
	got := tr.SeekOffset(10)
	want := int64(400000)
	want -= want % 960 // 帧对齐
	if got != want {
		t.Errorf("SeekOffset(10)=%d, want %d", got, want)
	}

	// 帧对齐：结果必须是 960 的整数倍
	for _, sec := range []float64{1, 7.3, 30, 123.456} {
		off := tr.SeekOffset(sec)
		if off%960 != 0 {
			t.Errorf("SeekOffset(%v)=%d 未对齐帧边界（%%960=%d）", sec, off, off%960)
		}
		if off < 0 {
			t.Errorf("SeekOffset(%v)=%d 为负", sec, off)
		}
	}

	// 单调性：秒越大偏移越大
	if !(tr.SeekOffset(5) < tr.SeekOffset(50)) {
		t.Error("SeekOffset 非单调递增")
	}
}
