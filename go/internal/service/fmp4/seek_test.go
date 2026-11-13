package fmp4

import (
	"encoding/binary"
	"os"
	"testing"
)

// 测试用的真实缓存 m4s（与 Python 原型同一文件）
const testPath = "/home/a1/.config/homecast/cache/audio/BV1ciWb6yEWW.m4s"

// tfdtOfSnippet 读取片段第一个 moof 的 tfdt（验证裁剪后时基）
func tfdtOfSnippet(b []byte) uint32 {
	p := int64(0)
	for p+8 <= int64(len(b)) {
		sz := int64(binary.BigEndian.Uint32(b[p : p+4]))
		typ := string(b[p+4 : p+8])
		if sz == 1 {
			sz = int64(binary.BigEndian.Uint64(b[p+8 : p+16]))
		}
		if sz < 8 || p+sz > int64(len(b)) {
			break
		}
		if typ == "moof" {
			// 内层：traf → tfdt
			sub := b[p+8 : p+sz]
			q := int64(0)
			for q+8 <= int64(len(sub)) {
				s2 := int64(binary.BigEndian.Uint32(sub[q : q+4]))
				t2 := string(sub[q+4 : q+8])
				if s2 < 8 || q+s2 > int64(len(sub)) {
					break
				}
				if t2 == "traf" {
					inner := sub[q+8 : q+s2]
					r := int64(0)
					for r+8 <= int64(len(inner)) {
						s3 := int64(binary.BigEndian.Uint32(inner[r : r+4]))
						t3 := string(inner[r+4 : r+8])
						if t3 == "tfdt" {
							ver := inner[r+8]
							if ver == 1 {
								return uint32(binary.BigEndian.Uint64(inner[r+12 : r+20]))
							}
							return binary.BigEndian.Uint32(inner[r+12 : r+16])
						}
						r += s3
					}
				}
				q += s2
			}
		}
		p += sz
	}
	return 0
}

func TestSeekBytesPrecision(t *testing.T) {
	if _, err := os.Stat(testPath); err != nil {
		t.Skip("无缓存 m4s, 跳过")
	}
	data, err := os.ReadFile(testPath)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		sec      float64
		wantSec  float64 // 期望输出时基（秒），容忍 ±0.03
		wantSize float64 // 期望输出相对大小（约等于源减去跳过的）
	}{
		{0.5, 0.5, 0.99}, {30.2, 30.2, 0.72}, {60.0, 60.0, 0.44}, {100.0, 100.0, 0.08},
	}
	for _, c := range cases {
		out, err := SeekBytes(data, c.sec)
		if err != nil {
			t.Fatalf("SeekBytes(%v) err=%v", c.sec, err)
		}
		gotSec := float64(tfdtOfSnippet(out)) / Timescale
		if gotSec < c.wantSec-0.03 || gotSec > c.wantSec+0.03 {
			t.Errorf("seek@%v: 时基 %.2fs 期望 ≈%.2f（超出 ±0.03）", c.sec, gotSec, c.wantSec)
		}
		if float64(len(out)) > float64(len(data))*c.wantSize+15000 {
			t.Errorf("seek@%v: 输出 %d 未显著小于源 %d", c.sec, len(out), len(data))
		}
	}
}

func TestSeekStartZero(t *testing.T) {
	data := []byte{1, 2, 3}
	out, err := SeekBytes(data, 0)
	if err != nil || len(out) != 3 || out[0] != 1 {
		t.Fatalf("start=0 应原样返回")
	}
}