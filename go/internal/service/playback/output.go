// Output 播放后端抽象：把"播放目标"（本机 web / 小爱音箱 / DLNA 电视）统一成
// 同一个接口，由播放核心（playback 包）统一调度。核心不再用 source 分支判断
// "现在是哪个设备"——只认 Output，装什么都能控制。
//
// 区域划分：
//   playback 包（统一核心）：状态 SSOT + 队列走位 + 控制语义 + 自动连播 + 持久化
//   Output 实现（各自自洽库）：只管"怎么真正出声 + 怎么上报进度"，不散落状态
package playback

import "errors"

// ErrUnsupported 输出不支持某能力（如小爱无法真 seek）。核心据此降级（忽略/置灰）。
var ErrUnsupported = errors.New("unsupported by output")

// Capabilities 输出能力声明：控制面据此渲染，不写死针对某个设备。
// Seek=false → 进度条置灰不可拖；Volume=false → 音量滑块置灰。
type Capabilities struct {
	Play     bool
	Pause    bool
	Resume   bool
	Stop     bool
	Next     bool
	Prev     bool
	Seek     bool
	Volume   bool
	Progress bool // 进度是否真实（false=不可知，进度条不显示）
}

// PlayOpts 播放参数（核心切歌时传给当前输出）
type PlayOpts struct {
	Title    string
	Duration float64 // 秒（前端已知最准；0 则输出端自行解析）
	NextBVID string  // 播完自动连播的下一首（循环模式核心填）
	Quality  int     // 音质（0=输出端默认）
}

// ProgressInfo 输出端当前进度（统一上报：带真实 bvid/title，音箱模式核心据此
// 对齐当前歌——音箱自动连播/蓝牙直播后核心与设备不再分裂）。web=Null 只有
// pos/dur（bvid 空，核心不覆盖——web 当前歌由前端 Play 命令定）。
type ProgressInfo struct {
	BVID   string  // 输出端实际在播的歌（音箱自报；web 空）
	Title  string  // 同上（歌词/显示用，避免核心旧标题）
	Pos    float64 // 秒
	Dur    float64 // 秒
	Playing bool
}

// Output 播放后端：装 web / 小爱 / DLNA 电视，核心只认这个接口。
//
// 契约：
//   - Play/Pause/Resume/Stop/Seek/SetVolume 负责"让设备真正响"，不改核心状态
//   - Position 负责"上报当前进度"（真实或模拟，由输出端自洽实现）
//   - SetEndedHook 让输出端在"播完"时回调核心（web: audio ended；小爱: 本地定时器到点）
type Output interface {
	// Play 播放指定歌（bvid 由核心传入；输出端自行把 bvid 转成真实音源）
	Play(bvid string, opts PlayOpts) error
	// Pause 暂停
	Pause() error
	// Resume 恢复
	Resume() error
	// Stop 停止
	Stop() error
	// Seek 定位（不支持则返回 ErrUnsupported，核心降级忽略）
	Seek(sec float64) error
	// SetVolume 音量（0-100）
	SetVolume(v int) error
	// Position 当前进度（真实或模拟：pos/dur/playing + 音箱自报 bvid/title）
	Position() ProgressInfo
	// Caps 能力声明
	Caps() Capabilities
	// SetEndedHook 注入"播完"回调（核心填自身 Next）
	SetEndedHook(fn func())
	// Name 输出标识（web/speaker/cast，日志用）
	Name() string
}

// NullOutput 空输出（无激活输出时占位：web 模式核心直接广播，前端 audio 响应）
type NullOutput struct{}

func (NullOutput) Play(string, PlayOpts) error  { return nil }
func (NullOutput) Pause() error                 { return nil }
func (NullOutput) Resume() error                { return nil }
func (NullOutput) Stop() error                  { return nil }
func (NullOutput) Seek(float64) error          { return nil }
func (NullOutput) SetVolume(int) error         { return nil }
func (NullOutput) Position() ProgressInfo         { return ProgressInfo{} }
func (NullOutput) Caps() Capabilities           { return Capabilities{} }
func (NullOutput) SetEndedHook(func())          {}
func (NullOutput) Name() string                 { return "null" }