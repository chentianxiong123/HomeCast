// Package fmp4 缓存 m4s（fragmented MP4）的容器级 seek 切流。
//
// 背景：音箱无 seek API（小米云 player_play_operation 只有 play/pause/resume/stop）。
// 续播/跳播的唯一可控办法 = 给音箱一个"从中间开始的合法流"：
// 缓存 m4s 是标准 fMP4（ftyp+moov init 段 + 一串独立 moof+mdat 片段，每片约 4.6s），
// 每个 fragment 内部是 AAC 帧序列（每帧 1024 采样 @48kHz = 21.33ms，帧独立可解）。
// 本包把目标秒定位到 fragment + fragment 内的 sample，重写该 moof
// （trun.sample_count 减少、tfdt 时基平移、data_offset 平移、mdat 数据跳过），
// 拼出 init + 裁剪后片段 + 后续原始片段 = 完全合法的音频流，从指定秒开始播。
// 纯容器操作，不转码（m4s 本就是音箱能解的 MP4+AAC）。
package fmp4

import (
	"encoding/binary"
	"fmt"
	"os"
)

// Timescale 音频时间尺度（tfdt/样本帧单位）。实测 m4s 为 48kHz（每样本 1024 帧）。
const Timescale = 48000

// SamplesPerFrame AAC 每帧采样数（fMP4 固定 1024）。
const SamplesPerFrame = 1024

// frag 单个 fragment 的定位信息
type frag struct {
	moofOff  int64 // moof 盒起始（相对文件）
	moofSize int64 // moof 盒尺寸
	mdatOff  int64 // mdat 盒起始
	mdatSize int64 // mdat 盒尺寸
	bt       int64 // tfdt baseMediaDecodeTime（媒体时基 @Timescale）
	// trun 关键字段（只裁剪 flags=0x201/0x200 型：data-offset+sample-size 表）
	trunOff    int64   // trun 盒起始
	trunSize   int64   // trun 盒尺寸
	flags      uint32  // trun flags（低24位）
	sampleCnt  uint32  // trun sample_count
	dataOffset int32   // trun data_offset（相对 moof 起点，flags&1）
	samples    []uint32 // sample-size 表（flags&0x200 时，顺序与 trun 一致）
	rowSize    int     // 样本表每行字节
}

// Seek 打开 path，返回从 startSec 开始的有效 fMP4 流字节。
// 定位精度 = AAC 帧粒度（21.33ms）。解析失败返回错误（调用方回退全量从头播）。
func Seek(path string, startSec float64) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return SeekBytes(data, startSec)
}

// SeekBytes 对已读入的 m4s 字节做 seek 切流（纯函数，便于测试）。
func SeekBytes(data []byte, startSec float64) ([]byte, error) {
	if startSec <= 0 {
		return data, nil // 从头：原样返回
	}
	initEnd, frags, err := parse(data)
	if err != nil {
		return nil, err
	}
	if len(frags) < 2 {
		return data, nil // 无足够片段可裁，原样
	}
	targetTS := int64(startSec * Timescale)
	// 定位：最后一个 bt<=target 的 fragment（其内部裁剪），且不是最末（后面还需有续段）
	idx := -1
	for i := range frags {
		if frags[i].bt <= targetTS && i < len(frags)-1 {
			idx = i
		}
	}
	if idx < 0 {
		// target 落在最末片段之后 → 前面没内容可丢，原样全量（音箱自己播到末尾停）
		return data, nil
	}
	f := &frags[idx]
	skip := int((targetTS - f.bt) / SamplesPerFrame)
	if skip > int(f.sampleCnt-1) {
		skip = int(f.sampleCnt - 1)
	}
	if skip <= 0 {
		// 目标正好在片段头或之前 → 从该片段完整开始，无需裁剪
		return data[f.moofOff:], nil
	}
	skipBytes := int64(0)
	for _, s := range f.samples[:skip] {
		skipBytes += int64(s)
	}
	// 重写 moof：trun sample_count -= skip、表头去掉 skip 行、data_offset 平移、
	// traf/moof 尺寸收缩、tfdt 时基平移 skip*SamplesPerFrame
	newMoof, err := rewriteMoof(data[f.moofOff:f.moofOff+f.moofSize], f, skip)
	if err != nil {
		return nil, err
	}
	// mdat：头尺寸修正 + 数据前跳 skipBytes
	newMdat := appendMdatHead(data[f.mdatOff:f.mdatOff+f.mdatSize], skipBytes)
	// 拼接：init + 裁剪后 moof + 裁剪后 mdat + 后续所有原始片段
	out := make([]byte, 0, initEnd+int64(len(newMoof))+int64(len(newMdat))+int64(len(data))-(f.mdatOff+f.mdatSize))
	out = append(out, data[:initEnd]...)
	out = append(out, newMoof...)
	out = append(out, newMdat...)
	out = append(out, data[f.mdatOff+f.mdatSize:]...)
	return out, nil
}

// rewriteMoof 重写 moof：裁剪 samples，修正 sample_count/data_offset/tfdt/各盒尺寸
func rewriteMoof(moof []byte, f *frag, skip int) ([]byte, error) {
	// 定位 traf 及其内部各子盒（moof: mfhd, traf{ tfhd, tfdt, trun }）
	trafOff, trafSize := boxIn(moof, 8, int64(len(moof)), "traf")
	if trafOff < 0 {
		return nil, fmt.Errorf("fmp4: moof 无 traf")
	}
	tfdtOff, tfdtSize := boxIn(moof, trafOff+8, trafOff+trafSize, "tfdt")
	if tfdtOff < 0 {
		return nil, fmt.Errorf("fmp4: traf 无 tfdt")
	}
	trunOff, trunSize := boxIn(moof, trafOff+8, trafOff+trafSize, "trun")
	if trunOff < 0 {
		return nil, fmt.Errorf("fmp4: traf 无 trun")
	}
	_ = tfdtSize
	// 新 trun：头部原样，sample_count 改小，样本表截去 skip 行
	trun := moof[trunOff : trunOff+trunSize]
	newTrun := make([]byte, 0, len(trun)-skip*f.rowSize)
	newTrun = append(newTrun, trun[:16]...) // 盒头+fullbox头+sample_count
	binary.BigEndian.PutUint32(newTrun[12:], f.sampleCnt-uint32(skip))
	// 头部可变部分（data_offset / first_sample_flags）先抄原样，改 size 后再算 data_offset
	headEnd := 16
	if f.flags&0x1 != 0 {
		headEnd += 4
	}
	if f.flags&0x4 != 0 {
		headEnd += 4
	}
	newTrun = append(newTrun, trun[16:headEnd]...)
	// 样本表：从 skip*f.rowSize 起
	newTrun = append(newTrun, trun[headEnd+skip*f.rowSize:]...)
	binary.BigEndian.PutUint32(newTrun, uint32(len(newTrun))) // trun 尺寸
	// 新 traf：head 到 trun 前 + 新 trun
	newTraf := make([]byte, 0, int64(len(trun))-int64(skip*f.rowSize))
	newTraf = append(newTraf, moof[trafOff:trunOff]...)
	newTraf = append(newTraf, newTrun...)
	binary.BigEndian.PutUint32(newTraf, uint32(len(newTraf)))
	// tfdt 时基平移（在 newTraf 头部区域内 → 对应位置）
	tfdtRel := tfdtOff - trafOff
	if t := newTraf[tfdtRel : tfdtRel+tfdtSize]; tfdtSize >= 20 {
		ver := t[8]
		shift := int64(skip * SamplesPerFrame)
		if ver == 1 {
			old := int64(binary.BigEndian.Uint64(t[12:20]))
			binary.BigEndian.PutUint64(t[12:20], uint64(old+shift))
		} else {
			old := int64(binary.BigEndian.Uint32(t[12:16]))
			binary.BigEndian.PutUint32(t[12:16], uint32(old+shift))
		}
	}
	// 新 moof：head 到 traf 前 + 新 traf
	newMoof := make([]byte, 0, int64(len(moof))-int64(skip*f.rowSize))
	newMoof = append(newMoof, moof[:trafOff]...)
	newMoof = append(newMoof, newTraf...)
	binary.BigEndian.PutUint32(newMoof, uint32(len(newMoof)))
	// data_offset 相对 moof 起点（fMP4 默认 base=moof 起点）：
	// 新样本数据起点 = 新moof长度 + mdat盒头(8)。原 mdat 头一律 8 字节（size≠1）。
	if f.flags&0x1 != 0 {
		// data_offset 字段在新 moof 内位置 = 新 trun 内偏移 16
		// trun 相对 moof 的偏移 = trun 在前缀内偏移（moof[:trafOff] + traf内trun前偏移）
		newTrunOff := int64(trafOff) + (trunOff - trafOff)
		binary.BigEndian.PutUint32(newMoof[newTrunOff+16:], uint32(len(newMoof)+8))
	}
	return newMoof, nil
}

// appendMdatHead 修正 mdat 头尺寸并截断数据前 skipBytes
func appendMdatHead(mdat []byte, skipBytes int64) []byte {
	if len(mdat) < 8+int(skipBytes) {
		return mdat
	}
	hdr := 8
	sz := int64(binary.BigEndian.Uint32(mdat[:4]))
	if sz == 1 {
		hdr = 16
		sz = int64(binary.BigEndian.Uint64(mdat[8:16]))
	}
	out := make([]byte, 0, len(mdat)-int(skipBytes))
	out = append(out, mdat[:hdr]...)
	if hdr == 16 {
		binary.BigEndian.PutUint64(out[8:16], uint64(sz-skipBytes))
	} else {
		binary.BigEndian.PutUint32(out[:4], uint32(sz-skipBytes))
	}
	out = append(out, mdat[hdr+int(skipBytes):]...)
	return out
}

// parse 解析 fMP4：init 段（ftyp+moov，sidx 丢弃）+ fragment 链表
func parse(data []byte) (initEnd int64, frags []frag, err error) {
	if len(data) < 8 {
		return 0, nil, fmt.Errorf("fmp4: 文件过短")
	}
	// init 段：ftyp+moov。sidx 丢弃（seek 后索引已失真；且 sidx 仅 DASH 随机访问用，顺序播放不需要）
	pos := int64(0)
	moofStart := int64(-1)
	for pos+8 <= int64(len(data)) {
		size, typ, ok := boxAt(data, pos)
		if !ok {
			return 0, nil, fmt.Errorf("fmp4: init 段解析失败 @%d", pos)
		}
		if typ == "moof" {
			moofStart = pos
			break
		}
		if typ == "sidx" {
			// init 停在 sidx 前（sidx 丢弃）；moofStart 尚未找到，继续
			pos += size
			continue
		}
		initEnd = pos + size
		pos += size
	}
	if moofStart < 0 {
		return 0, nil, fmt.Errorf("fmp4: 无 moof")
	}
	// boxAtFull 提前 ok 校验：直接读 moof（偏移即 p）
	p := moofStart
	for p+8 <= int64(len(data)) {
		mSize, mtyp, moofOK := boxAtFull(data, p)
		if !moofOK || mtyp != "moof" {
			break
		}
		moff := p
		f := frag{moofOff: moff, moofSize: mSize}
		// mdat 紧随
		md := moff + mSize
		if md+8 > int64(len(data)) {
			break
		}
		mdSize, _, ok := boxAtFull(data, md)
		if !ok || mdSize < 8 {
			break
		}
		f.mdatOff = md
		f.mdatSize = mdSize
		// 读 tfdt / trun
		if err := parseMoof(data, &f); err != nil {
			return 0, nil, err
		}
		frags = append(frags, f)
		p = md + mdSize
		if len(frags) > 10000 {
			break
		}
	}
	if len(frags) == 0 {
		return 0, nil, fmt.Errorf("fmp4: 无 fragment")
	}
	return initEnd, frags, nil
}

// parseMoof 解析单个 moof 的 tfdt 和 trun
func parseMoof(data []byte, f *frag) error {
	moof := data[f.moofOff : f.moofOff+f.moofSize]
	trafOff, trafSize := boxIn(moof, 8, int64(len(moof)), "traf")
	if trafOff < 0 {
		return fmt.Errorf("fmp4: moof 无 traf")
	}
	// tfdt
	if off, sz := boxIn(moof, trafOff+8, trafOff+trafSize, "tfdt"); off >= 0 && sz >= 16 {
		t := moof[off : off+sz]
		if t[8] == 1 {
			f.bt = int64(binary.BigEndian.Uint64(t[12:20]))
		} else {
			f.bt = int64(binary.BigEndian.Uint32(t[12:16]))
		}
	} else {
		return fmt.Errorf("fmp4: traf 无 tfdt")
	}
	// trun
	trunOff, trunSize := boxIn(moof, trafOff+8, trafOff+trafSize, "trun")
	if trunOff < 0 {
		return fmt.Errorf("fmp4: traf 无 trun")
	}
	f.trunOff, f.trunSize = trunOff, trunSize
	trun := moof[trunOff : trunOff+trunSize]
	if len(trun) < 16 {
		return fmt.Errorf("fmp4: trun 过短")
	}
	f.flags = binary.BigEndian.Uint32(trun[8:12]) & 0xffffff
	f.sampleCnt = binary.BigEndian.Uint32(trun[12:16])
	p := 16
	if f.flags&0x1 != 0 {
		if len(trun) < 20 {
			return fmt.Errorf("fmp4: trun 无 data_offset")
		}
		f.dataOffset = int32(binary.BigEndian.Uint32(trun[16:20]))
		p += 4
	}
	if f.flags&0x4 != 0 {
		p += 4
	}
	rows := 0
	if f.flags&0x100 != 0 {
		rows += 4
	}
	if f.flags&0x200 != 0 {
		rows += 4
	}
	if f.flags&0x400 != 0 {
		rows += 4
	}
	if f.flags&0x800 != 0 {
		rows += 4
	}
	f.rowSize = rows
	// 读 sample-size 表（仅当 size 字段存在；注释：Go 语法调整见下）
	if f.flags&0x200 != 0 {
		_ = rows
		f.samples = make([]uint32, 0, f.sampleCnt)
		for i := uint32(0); i < f.sampleCnt && p+4 <= len(trun); i++ {
			var q = p
			if f.flags&0x100 != 0 {
				q += 4
			}
			f.samples = append(f.samples, binary.BigEndian.Uint32(trun[q:q+4]))
			p += rows
		}
	}
	return nil
}

// boxAt 读取 pos 处盒子（不跨 8/16 字节头）
func boxAt(b []byte, pos int64) (size int64, typ string, ok bool) {
	if pos+8 > int64(len(b)) {
		return 0, "", false
	}
	sz := int64(binary.BigEndian.Uint32(b[pos : pos+4]))
	t := string(b[pos+4 : pos+8])
	if sz == 1 {
		if pos+16 > int64(len(b)) {
			return 0, "", false
		}
		sz = int64(binary.BigEndian.Uint64(b[pos+8 : pos+16]))
	}
	if sz < 8 || pos+sz > int64(len(b)) {
		return 0, "", false
	}
	return sz, t, true
}

// boxAtFull 同 boxAt，但额外返回是否合法（用于 mdat 等）
func boxAtFull(b []byte, pos int64) (size int64, typ string, ok bool) { return boxAt(b, pos) }

// boxIn 在 [start,end) 范围找 type 盒，返回其偏移和尺寸
func boxIn(b []byte, start, end int64, want string) (int64, int64) {
	p := start
	for p+8 <= end && p+8 <= int64(len(b)) {
		sz, typ, ok := boxAt(b, p)
		if !ok {
			return -1, 0
		}
		if typ == want {
			return p, sz
		}
		p += sz
	}
	return -1, 0
}