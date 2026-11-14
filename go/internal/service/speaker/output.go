// SpeakerOutput 音箱播放后端：小爱能力（miio/ubus 投送 + 暂停/恢复/切歌/音量）的唯一对接点。
// 实现 playback.Output 接口——播放核心只认 Output，不再 source 分支判断（对接归位）。
//
// 契约（"一切以我为准"）：
//   - 状态完全本地自洽：进度 = 本地模拟时钟（SpeakerProgress），不查音箱（音箱不报真实状态）
//   - 音箱是哑执行器：只接收播放/暂停/停止/音量命令，不读它的状态
//   - 暂停/恢复命令必须真送到音箱（实测 player_stop 能停；player_pause 见实测）
//   - 进度：本地时钟 + 暂停补偿（pauseProgress/resumeProgress 已有）
package speaker

import (
	"fmt"
	"sync"

	"homecast/internal/service/playback"
)

// SpeakerOutput 绑定某台音箱（did），实现 playback.Output
type SpeakerOutput struct {
	svc   *SpeakerService
	did   string
	mu    sync.Mutex
	onEnded func() // 播完回调（核心注入：自动连播）
}

// NewSpeakerOutput 构造绑定 did 的音箱输出
func NewSpeakerOutput(svc *SpeakerService, did string) *SpeakerOutput {
	return &SpeakerOutput{svc: svc, did: did}
}

// Play 投送/切歌：bvid → 代理流 → 音箱播放（复用 SpeakerService 全链路），并设自动连播钩子
func (o *SpeakerOutput) Play(bvid string, opts playback.PlayOpts) error {
	q := opts.Quality
	if q == 0 {
		q = 30280 // 音箱默认 30280（B站 DASH 音频，xiaomusic 同款）
	}
	res, err := o.svc.Play(bvid, o.did, q, PlayOpts{
		Title:    opts.Title,
		Duration: opts.Duration,
		NextBVID: opts.NextBVID,
	})
	if err != nil {
		return err
	}
	if c, _ := res["code"].(int); c != 0 {
		msg, _ := res["message"].(string)
		return fmt.Errorf("音箱投送失败 code=%d %s", c, msg)
	}
	return nil
}

// Pause 暂停（真调音箱 player_pause；"一切以我为准"，状态本地已由核心设 false）
func (o *SpeakerOutput) Pause() error {
	res, err := o.svc.Control(o.did, "pause", 0)
	if err != nil {
		return err
	}
	if c, _ := res["code"].(int); c != 0 {
		return fmt.Errorf("音箱暂停失败 code=%d", c)
	}
	return nil
}

// Resume 恢复（真调音箱 player_resume + 本地进度补偿）
func (o *SpeakerOutput) Resume() error {
	res, err := o.svc.Control(o.did, "resume", 0)
	if err != nil {
		return err
	}
	if c, _ := res["code"].(int); c != 0 {
		return fmt.Errorf("音箱恢复失败 code=%d", c)
	}
	return nil
}

// Stop 停止
func (o *SpeakerOutput) Stop() error {
	res, err := o.svc.Control(o.did, "stop", 0)
	if err != nil {
		return err
	}
	if c, _ := res["code"].(int); c != 0 {
		return fmt.Errorf("音箱停止失败 code=%d", c)
	}
	return nil
}

// Seek 小爱无法真 seek（缓冲模式/码率不可知）——返回不支持，核心降级（进度条置灰）
func (o *SpeakerOutput) Seek(sec float64) error { return playback.ErrUnsupported }

// SetVolume 音量（真调音箱）
func (o *SpeakerOutput) SetVolume(v int) error {
	res, err := o.svc.SetVolume(o.did, v)
	if err != nil {
		return err
	}
	if c, _ := res["code"].(int); c != 0 {
		return fmt.Errorf("音箱音量失败 code=%d", c)
	}
	return nil
}

// Volume 回读音箱当前真实音量（0-100）。切音箱时调一次对齐前端滑块/静音态（否则滑块
// 靠猜 localStorage，与设备实际脱节）。仅切音箱时读一次，非进度轮询（不违反"不读状态"契约——
// 那契约指进度；音量是设备设置项，回读一次对齐 UI 合理）。失败返回 -1（调用方降级用默认）。
func (o *SpeakerOutput) Volume() int {
	status, err := o.svc.GetStatus(o.did)
	if err != nil {
		return -1
	}
	if c, _ := status["code"].(int); c != 0 {
		return -1
	}
	data, _ := status["data"].(map[string]any)
	if data == nil {
		return -1
	}
	if v, ok := data["volume"].(float64); ok {
		return int(v)
	}
	if v, ok := data["volume"].(int); ok {
		return v
	}
	return -1
}

// Position 音箱当前进度（本地模拟；bvid/title 取自音箱自身进度——核心据此
// 对齐当前歌，自动连播/外部投送后核心与音箱不分裂）
func (o *SpeakerOutput) Position() playback.ProgressInfo {
	d := o.svc.Progress(o.did)
	bvid, _ := d["bvid"].(string)
	title, _ := d["title"].(string)
	pos, _ := d["position"].(float64)
	dur, _ := d["duration"].(float64)
	return playback.ProgressInfo{
		BVID:    bvid,
		Title:   title,
		Pos:     pos,
		Dur:     dur,
		Playing: d["state"] == "playing",
	}
}

// Caps 能力声明（小爱唯一不支持：真 seek；进度为本地模拟非真实）
func (o *SpeakerOutput) Caps() playback.Capabilities {
	return playback.Capabilities{
		Play: true, Pause: true, Resume: true, Stop: true,
		Next: true, Prev: true, Seek: false, Volume: true, Progress: true,
	}
}

// SetEndedHook 注入播完回调（核心自动连播用）
func (o *SpeakerOutput) SetEndedHook(fn func()) {
	o.mu.Lock()
	o.onEnded = fn
	o.mu.Unlock()
}

// ended 播完（SpeakerService 本地定时器到点回调）→ 转发核心
func (o *SpeakerOutput) ended() {
	o.mu.Lock()
	fn := o.onEnded
	o.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// Name 输出标识
func (o *SpeakerOutput) Name() string { return "speaker" }
