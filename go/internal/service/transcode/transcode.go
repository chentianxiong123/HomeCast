// Package transcode 音频软转码域：缓存 m4s(fMP4) → 320k CBR mp3，供音箱"秒开 + 字节 seek 续播"。
//
// 根因（实测见 docs/2026-11-14-音箱出声延迟根因）：B站音频源是 fMP4/m4s 容器，音箱"边下边喂"
// 时要凑够 moov 头 + 连续可解码片段才出声（实测拉满 48 次/13MB 才出声 → 三四秒延迟）。
// mp3 是流式帧格式，音箱拿到第一段即解码出声。故喂音箱前把 m4s 软转码成 mp3。
//
// 为什么 320k CBR：
//   - 320k 是 MP3 CBR 上限，B站 AAC 源（最高档 ~192k）任一档都完整承载，零降质
//   - CBR 码率恒定 → 续播"秒↔字节"可线性换算（字节 = 秒 × 码率/8），无需逐帧索引
//   - VBR 秒↔字节非线性，seek 要建索引，不采用
//
// 缓存策略（对齐用户要求）：转完再缓存——同步转码整首 → 落盘完整 mp3 → 缓存；
// 不缓存"边转边喂"的流形态。首次转码几秒（纯 CPU 几十倍实时），同曲二次秒开。
// 音箱续播（无 seek API）= 给它一段"从指定字节起的完整剩余 mp3"（200 全量，踩
// "200 全量稳定 / 206 分片炸音"铁证），起始字节由 SeekOffset 按 CBR 线性换算。
package transcode

import (
	"bytes"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"homecast/internal/service/conv"
)

// Table mp3 缓存元数据表（独立于源 m4s 的 audio_cache；键 bvid+quality）
const Table = "audio_mp3_cache"

// Schema mp3 缓存表定义。bitrate 记转码码率（供 seek 换算读实际值，不硬编码）；
// frame_bytes 记实测帧大小（转码后校准，避免推算误差致 seek 落帧中间）。
const Schema = `CREATE TABLE IF NOT EXISTS audio_mp3_cache (
	bvid TEXT NOT NULL, quality INTEGER NOT NULL, size INTEGER DEFAULT 0,
	bitrate INTEGER DEFAULT 0, frame_bytes INTEGER DEFAULT 0,
	last_used INTEGER DEFAULT 0, created_at INTEGER DEFAULT 0,
	PRIMARY KEY(bvid, quality))`

// limitKey settings kv 键（mp3 缓存上限，独立于 m4s 上限）
const limitKey = "mp3_cache_limit_mb"

// DefaultLimitMB 默认 mp3 缓存上限（MB）；0=关闭
const DefaultLimitMB = 800

// CBR 转码目标码率（bit/s）。320k = MP3 CBR 上限。
const CBRBitrate = 320000

// bytesPerSec 每秒字节数 = 码率/8
const bytesPerSec = CBRBitrate / 8 // 40000

// Transcoder 软转码 + mp3 缓存（SQLite 元数据 + 磁盘文件）
type Transcoder struct {
	mu       sync.Mutex
	db       *sql.DB
	mp3Dir   string // dataDir/cache/audio-mp3
	limitMB  int    // 后端持有并持久化（0=关）
	ffmpegOK bool   // 启动时探测 ffmpeg 可用性；不可用则转码功能降级关闭
}

// NewTranscoder 构造。mp3Dir 为 mp3 缓存目录；limitMB 初值（<0 用默认/读 settings）。
// 启动探测 ffmpeg（转码功能的硬依赖；缺失则 Available()=false，调用方降级回原 m4s 路径）。
func NewTranscoder(db *sql.DB, mp3Dir string, limitMB int) *Transcoder {
	t := &Transcoder{db: db, mp3Dir: mp3Dir, limitMB: DefaultLimitMB}
	if limitMB >= 0 {
		t.limitMB = limitMB
	} else if v, err := conv.LoadLimit(db, limitKey, DefaultLimitMB); err == nil {
		t.limitMB = v
	}
	t.ffmpegOK = probeFFmpeg()
	if !t.ffmpegOK {
		log.Printf("[transcode] ffmpeg 不可用 → 音箱秒开转码降级关闭（回退原 m4s 路径）")
	}
	t.cleanupTmp()
	return t
}

// Available ffmpeg 是否可用（转码功能开关）
func (t *Transcoder) Available() bool { return t.ffmpegOK }

// probeFFmpeg 探测 ffmpeg 是否在 PATH（转码硬依赖）
func probeFFmpeg() bool {
	return exec.Command("ffmpeg", "-version").Run() == nil
}

// cleanupTmp 启动清 .tmp 残留（上次进程被杀，转码未走完 rename 的孤儿）
func (t *Transcoder) cleanupTmp() {
	entries, err := os.ReadDir(t.mp3Dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			_ = os.Remove(filepath.Join(t.mp3Dir, e.Name()))
		}
	}
}

// FilePath mp3 缓存文件路径（按 bvid+quality 区分音源档）
func (t *Transcoder) FilePath(bvid string, quality int) string {
	return filepath.Join(t.mp3Dir, fmt.Sprintf("%s-%d.mp3", bvid, quality))
}

// Get 取 mp3 缓存（命中刷新 last_used=LRU 触碰）；文件不存在返回 false
func (t *Transcoder) Get(bvid string, quality int) (size int64, ok bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	path := t.FilePath(bvid, quality)
	fi, err := os.Stat(path)
	if err != nil || fi.Size() == 0 {
		return 0, false
	}
	_, _ = t.db.Exec(`UPDATE audio_mp3_cache SET last_used=? WHERE bvid=? AND quality=?`,
		time.Now().UnixMilli(), bvid, quality)
	return fi.Size(), true
}

// EnsureMP3 确保 mp3 已转码并缓存，返回 mp3 路径（供 handler serve）。
// 缓存命中直接返回；未命中从 m4sPath 同步转码整首 → temp 落盘 → rename 缓存。
// 并发同曲转码用 mu 串行（转码 CPU 密集，避免同曲多份 ffmpeg 互相踩）。
func (t *Transcoder) EnsureMP3(m4sPath, bvid string, quality int) (string, error) {
	if !t.ffmpegOK {
		return "", fmt.Errorf("ffmpeg 不可用")
	}
	if _, ok := t.Get(bvid, quality); ok {
		return t.FilePath(bvid, quality), nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	// 双检（等锁期间可能已被并发请求转好）
	if fi, err := os.Stat(t.FilePath(bvid, quality)); err == nil && fi.Size() > 0 {
		return t.FilePath(bvid, quality), nil
	}
	if err := os.MkdirAll(t.mp3Dir, 0o755); err != nil {
		return "", err
	}
	final := t.FilePath(bvid, quality)
	tmp := final + ".tmp"
	// -write_xing 0：不写 Xing/VBR 头（头部可变长度会干扰字节偏移计算；CBR 不需要）
	// -id3v2_version 0：不写 ID3v2 头（同理）；尾部 ID3v1 固定 128B 在末尾不影响 seek
	// -b:a 320k：CBR 320k；-vn：丢弃封面视频流（m4s 可能带图）；采样率跟随源（B站 48k）
	cmd := exec.Command("ffmpeg", "-y", "-hide_banner", "-loglevel", "error",
		"-i", m4sPath,
		"-vn", "-c:a", "libmp3lame", "-b:a", strconv.Itoa(CBRBitrate/1000)+"k",
		"-write_xing", "0", "-id3v2_version", "0",
		"-f", "mp3", tmp)
	var eb bytes.Buffer
	cmd.Stderr = &eb
	t0 := time.Now()
	if err := cmd.Run(); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("ffmpeg 转码失败: %v %s", err, eb.String())
	}
	fi, err := os.Stat(tmp)
	if err != nil || fi.Size() == 0 {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("转码产物为空")
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	// 写元数据（size + bitrate 供 seek 换算）+ LRU 淘汰
	_, _ = t.db.Exec(`INSERT INTO audio_mp3_cache (bvid,quality,size,bitrate,frame_bytes,last_used,created_at)
		VALUES (?,?,?,?,?,?,?) ON CONFLICT(bvid,quality) DO UPDATE SET size=?, bitrate=?, last_used=?`,
		bvid, quality, fi.Size(), CBRBitrate, 0, time.Now().UnixMilli(), time.Now().UnixMilli(),
		fi.Size(), CBRBitrate, time.Now().UnixMilli())
	t.evictLocked()
	log.Printf("[transcode] %s q=%d 转码完成 %dB 耗时 %dms", bvid, quality, fi.Size(), time.Since(t0).Milliseconds())
	return final, nil
}

// SeekOffset 续播起始字节偏移（320k CBR 线性换算 + 帧边界向下对齐）。startSec<=0 返回 0（从头）。
// 字节 = 秒 × 码率/8；帧对齐防落在帧中间（解码器虽能自同步，对齐更干净）。
// 帧大小按 MPEG-1 Layer III @48kHz 推算 ≈960B（144×320000/48000）；转码后可由实测校准
// （frame_bytes 列，当前留 0 用推算值；真测若有 seek 处杂音 → 用 ffprobe 校准此值）。
func (t *Transcoder) SeekOffset(startSec float64) int64 {
	if startSec <= 0 {
		return 0
	}
	off := int64(startSec * float64(bytesPerSec))
	const frameBytes = 960
	off -= off % frameBytes
	return off
}

// evictLocked LRU 淘汰：总量超上限删最旧 last_used（需持锁）
func (t *Transcoder) evictLocked() {
	if t.limitMB <= 0 {
		return
	}
	limitBytes := int64(t.limitMB) * 1024 * 1024
	var total int64
	_ = t.db.QueryRow(`SELECT COALESCE(SUM(size),0) FROM audio_mp3_cache`).Scan(&total)
	if total <= limitBytes {
		return
	}
	rows, err := t.db.Query(`SELECT bvid, quality FROM audio_mp3_cache ORDER BY last_used ASC`)
	if err != nil {
		return
	}
	type key struct {
		bvid    string
		quality int
	}
	var victims []key
	for rows.Next() {
		var k key
		if rows.Scan(&k.bvid, &k.quality) == nil {
			victims = append(victims, k)
		}
	}
	rows.Close()
	for _, k := range victims {
		if total <= limitBytes {
			break
		}
		var sz int64
		_ = t.db.QueryRow(`SELECT size FROM audio_mp3_cache WHERE bvid=? AND quality=?`, k.bvid, k.quality).Scan(&sz)
		_, _ = t.db.Exec(`DELETE FROM audio_mp3_cache WHERE bvid=? AND quality=?`, k.bvid, k.quality)
		_ = os.Remove(t.FilePath(k.bvid, k.quality))
		total -= sz
	}
}
