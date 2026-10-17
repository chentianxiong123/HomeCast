// API 基础地址统一出口
// - dev：走 vite proxy（相对路径，'' 前缀）
// - 生产（Wails 桌面壳 / 静态托管）：内嵌后端绝对地址
// - 可被 window.__HC_BACKEND__ 覆盖（外部注入场景）
export const API_BASE: string =
  (window as any).__HC_BACKEND__ || (import.meta.env.DEV ? '' : 'http://127.0.0.1:28976')