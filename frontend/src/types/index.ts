// 音乐项
export interface MusicItem {
  bvid: string
  title: string
  artist: string
  cover: string
  duration: string
  duration_sec: number
  play_count: number
}

// 视频信息
export interface VideoInfo {
  bvid: string
  aid: number
  cid: number
  title: string
  desc: string
  cover: string
  duration: number
  artist: string
  artist_id: number
}

// 音频流
export interface AudioStream {
  url: string
  quality: number
  size: number
  mime_type: string
  codecs: string
  cached: boolean
}

// 播放列表项
export interface PlaylistItem extends MusicItem {
  id?: number
  sort_order?: number
  added_at?: string
}

// 播放模式
export type PlayMode = 'order' | 'loop' | 'single' | 'random'

// 播放器状态
export interface PlayerState {
  isPlaying: boolean
  isLoading: boolean
  currentTime: number
  duration: number
  volume: number
  isMuted: boolean
  playMode: PlayMode
  currentSong: MusicItem | null
}

// API 响应
export interface ApiResponse<T> {
  code: number
  message: string
  data: T
}

// 搜索结果
export interface SearchResult {
  total: number
  list: MusicItem[]
}
