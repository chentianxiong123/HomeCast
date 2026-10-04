import request from './request'

// 桌面歌词挂件状态上报（WebView → 内嵌后端 → 同进程挂件读取）
export interface WidgetReport {
  bvid: string
  title: string
  artist: string
  current_time: number
  duration: number
  playing: boolean
}

export async function reportWidgetState(st: WidgetReport) {
  try {
    await request.post('/widget/state', st)
  } catch {
    // 上报失败不影响播放
  }
}

let lastReport = 0
const REPORT_INTERVAL = 1000

// 节流上报（播放中 1s 粒度，状态跳变立即报）
export function reportWidgetThrottled(get: () => WidgetReport) {
  const now = Date.now()
  if (now - lastReport >= REPORT_INTERVAL || !get().playing) {
    lastReport = now
    reportWidgetState(get())
  }
}

// 立即上报（切歌/暂停/恢复）
export function reportWidgetNow(get: () => WidgetReport) {
  lastReport = Date.now()
  reportWidgetState(get())
}