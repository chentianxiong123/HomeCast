import { ref, computed, watch } from 'vue'
import type { MusicItem, PlayerState, PlayMode } from '@/types'
import { getAudioStream } from '@/api/music'
import { getPlaylist, addToPlaylist as apiAddToPlaylist, removeFromPlaylist as apiRemoveFromPlaylist, clearPlaylist as apiClearPlaylist } from '@/api/playlist'

const VOLUME_KEY = 'bilibili-music-volume'
const MODE_KEY = 'bilibili-music-mode'
const PROGRESS_KEY = 'bilibili-music-progress'

// 存储接口
interface Storage {
  get(key: string): Promise<string | null>
  set(key: string, value: string): Promise<void>
}

// 优先使用 Preferences，不可用则回退到 localStorage
let storage: Storage = {
  async get(key: string) {
    try {
      const { Preferences } = await import('@capacitor/preferences')
      const result = await Preferences.get({ key })
      return result.value
    } catch {
      return localStorage.getItem(key)
    }
  },
  async set(key: string, value: string) {
    try {
      const { Preferences } = await import('@capacitor/preferences')
      await Preferences.set({ key, value })
    } catch {
      localStorage.setItem(key, value)
    }
  }
}

// 音频/视频元素
let mediaElement: HTMLAudioElement | HTMLVideoElement | null = null

// 播放器状态
const state = ref<PlayerState>({
  isPlaying: false,
  isLoading: false,
  currentTime: 0,
  duration: 0,
  volume: 80,
  isMuted: false,
  playMode: 'loop' as PlayMode,
  currentSong: null
})

// 播放列表
const playlist = ref<MusicItem[]>([])
const currentIndex = ref(0)
let isInitialized = false

// 从后端加载播放列表
export async function loadPlaylistFromBackend() {
  try {
    const list = await getPlaylist()
    playlist.value = list
    console.log('Loaded playlist from backend:', list.length, 'songs')

    // 如果有播放列表但没有当前歌曲，设置第一首为当前歌曲
    if (playlist.value.length > 0 && !state.value.currentSong) {
      state.value.currentSong = playlist.value[0]
      console.log('Set current song:', state.value.currentSong?.title)
    }
  } catch (e) {
    console.error('Failed to load playlist from backend:', e)
  }
}

// 初始化函数
export async function initPlayer() {
  if (isInitialized) return
  isInitialized = true

  console.log('Initializing player...')

  const [volumeData, modeData] = await Promise.all([
    storage.get(VOLUME_KEY),
    storage.get(MODE_KEY)
  ])

  if (volumeData) {
    state.value.volume = parseInt(volumeData, 10)
  }

  if (modeData) {
    state.value.playMode = modeData as PlayMode
  }

  // 从后端加载播放列表
  await loadPlaylistFromBackend()

  // 不预创建媒体元素，根据播放内容动态创建
  console.log('Player initialized')

  console.log('Player initialized')
}

// 初始化媒体元素（audio 或 video）
function initMediaElement(useVideo: boolean = false) {
  // 如果已有元素且类型匹配，复用（只重置src）
  if (mediaElement) {
    const isVideo = mediaElement instanceof HTMLVideoElement
    if (useVideo === isVideo) {
      // 同类型，复用，先暂停但不移除
      mediaElement.pause()
      return
    }
    // 类型不匹配，销毁旧元素
    mediaElement.pause()
    mediaElement.src = ''
    mediaElement.load()
    mediaElement.remove()
    mediaElement = null
  }

  console.log(`Creating ${useVideo ? 'video' : 'audio'} element...`)

  if (useVideo) {
    // 创建 video 元素播放 B站 MP4 流
    const video = document.createElement('video')
    video.style.width = '0px'
    video.style.height = '0px'
    video.style.position = 'absolute'
    video.style.opacity = '0'
    video.style.pointerEvents = 'none'
    video.preload = 'auto'
    video.playsInline = true
    document.body.appendChild(video)
    mediaElement = video
  } else {
    // 创建 audio 元素播放 MP3 缓存
    const audio = document.createElement('audio')
    audio.style.display = 'none'
    audio.preload = 'auto'
    document.body.appendChild(audio)
    mediaElement = audio
  }

  // 绑定事件（只绑定一次）
  mediaElement.addEventListener('loadedmetadata', () => {
    console.log('Media metadata loaded, duration:', mediaElement?.duration)
    state.value.duration = mediaElement?.duration || 0
    state.value.isLoading = false
  })

  mediaElement.addEventListener('timeupdate', () => {
    if (mediaElement) {
      state.value.currentTime = mediaElement.currentTime
    }
  })

  mediaElement.addEventListener('ended', () => {
    console.log('Media ended')
    handleSongEnd()
  })

  mediaElement.addEventListener('error', (e) => {
    const el = e.target as HTMLMediaElement
    console.error('Media error:', el.error?.code, el.error?.message)
    state.value.isLoading = false
    state.value.isPlaying = false
  })

  mediaElement.addEventListener('waiting', () => {
    console.log('Media waiting...')
  })

  mediaElement.addEventListener('playing', () => {
    console.log('Media playing')
    state.value.isLoading = false
    state.value.isPlaying = true
  })

  mediaElement.addEventListener('canplay', () => {
    console.log('Media can play')
    state.value.isLoading = false
  })

  // 保存播放进度
  let progressSaveTimeout: number | null = null
  mediaElement.addEventListener('timeupdate', () => {
    if (mediaElement && state.value.currentSong) {
      state.value.currentTime = mediaElement.currentTime

      if (progressSaveTimeout) clearTimeout(progressSaveTimeout)
      progressSaveTimeout = window.setTimeout(() => {
        if (state.value.currentSong && mediaElement) {
          storage.set(`${PROGRESS_KEY}-${state.value.currentSong.bvid}`, mediaElement.currentTime.toString())
        }
      }, 5000)
    }
  })

  mediaElement.volume = state.value.volume / 100
  console.log('Media element created')
}

// 播放指定歌曲
export async function play(song: MusicItem) {
  console.log('Play called with:', song.title, song.bvid)

  if (!isInitialized) await initPlayer()

  state.value.isLoading = true
  state.value.currentSong = song

  // 确保歌曲在播放列表中，并设置为当前播放
  const existingIndex = playlist.value.findIndex(s => s.bvid === song.bvid)
  if (existingIndex >= 0) {
    currentIndex.value = existingIndex
  } else {
    playlist.value.push(song)
    currentIndex.value = playlist.value.length - 1
  }

  try {
    // 后端 /stream/{bvid} 直接返回音频流（代理B站音频）
    const audioUrl = `/api/v1/music/stream/${song.bvid}?quality=64`
    console.log('Playing stream URL:', audioUrl)

    // 后端返回MP3格式，使用audio元素播放
    initMediaElement(false)

    // 重置并加载新源
    mediaElement!.src = audioUrl
    mediaElement!.load()

    // 等待可以播放
    await new Promise<void>((resolve, reject) => {
      const onCanPlay = () => {
        mediaElement!.removeEventListener('canplay', onCanPlay)
        mediaElement!.removeEventListener('error', onError)
        resolve()
      }
      const onError = (e: Event) => {
        mediaElement!.removeEventListener('canplay', onCanPlay)
        mediaElement!.removeEventListener('error', onError)
        const el = e.target as HTMLMediaElement
        reject(new Error(`Media load error: ${el.error?.code} ${el.error?.message}`))
      }
      if (mediaElement!.readyState >= 3) {
        resolve()
      } else {
        mediaElement!.addEventListener('canplay', onCanPlay)
        mediaElement!.addEventListener('error', onError)
      }
    })

    const playPromise = mediaElement!.play()
    if (playPromise !== undefined) {
      playPromise.catch(err => {
        console.warn('Play interrupted:', err)
      })
    }

    state.value.isPlaying = true
    state.value.isLoading = false

    storage.get(`${PROGRESS_KEY}-${song.bvid}`).then(savedProgress => {
      if (savedProgress && mediaElement && mediaElement.currentTime === 0) {
        const progress = parseFloat(savedProgress)
        if (!isNaN(progress) && progress > 0) {
          mediaElement.currentTime = progress
        }
      }
    })
  } catch (error) {
    console.error('Play failed:', error)
    state.value.isPlaying = false
    state.value.isLoading = false
  }
}

// 播放/暂停切换
export function togglePlay() {
  if (!mediaElement || !state.value.currentSong) {
    console.log('Toggle play skipped: no media or song')
    return
  }

  if (state.value.isPlaying) {
    mediaElement.pause()
    state.value.isPlaying = false
  } else {
    mediaElement.play().catch((err) => {
      console.error('Toggle play error:', err)
    })
  }
}

// 暂停
export function pause() {
  if (mediaElement) {
    mediaElement.pause()
    state.value.isPlaying = false
  }
}

// 跳转到指定时间
export function seekTo(time: number) {
  if (mediaElement) {
    mediaElement.currentTime = time
    state.value.currentTime = time
  }
}

// 跳转到指定百分比
export function seekToPercent(percent: number) {
  if (mediaElement && state.value.duration) {
    const time = (percent / 100) * state.value.duration
    seekTo(time)
  }
}

// 设置音量
export function setVolume(volume: number) {
  const v = Math.max(0, Math.min(100, volume))
  state.value.volume = v
  state.value.isMuted = v === 0
  if (mediaElement) {
    mediaElement.volume = v / 100
  }
}

// 静音切换
export function toggleMute() {
  if (!mediaElement) return

  if (state.value.isMuted) {
    mediaElement.volume = state.value.volume / 100
    state.value.isMuted = false
  } else {
    mediaElement.volume = 0
    state.value.isMuted = true
  }
}

// 添加到播放列表（同步到后端）
export async function addToPlaylist(songs: MusicItem | MusicItem[]) {
  const songsArray = Array.isArray(songs) ? songs : [songs]
  const existingBvids = new Set(playlist.value.map(s => s.bvid))
  const newSongs = songsArray.filter(s => !existingBvids.has(s.bvid))

  for (const song of newSongs) {
    try {
      const added = await apiAddToPlaylist(song.bvid)
      if (added) {
        playlist.value.push(added)
      }
    } catch (e) {
      console.error('Failed to add song to backend playlist:', e)
    }
  }
}

// 清空播放列表（同步到后端）
export async function clearPlaylist() {
  try {
    await apiClearPlaylist()
  } catch (e) {
    console.error('Failed to clear backend playlist:', e)
  }
  playlist.value = []
  currentIndex.value = 0
  state.value.currentSong = null
  state.value.isPlaying = false
  if (mediaElement) {
    mediaElement.src = ''
  }
}

// 播放下一首
export function next() {
  if (playlist.value.length === 0) return

  switch (state.value.playMode) {
    case 'single':
      if (state.value.currentSong) {
        play(state.value.currentSong)
      }
      return

    case 'random':
      currentIndex.value = Math.floor(Math.random() * playlist.value.length)
      break

    case 'loop':
    case 'order':
    default:
      if (currentIndex.value < playlist.value.length - 1) {
        currentIndex.value++
      } else if (state.value.playMode === 'loop') {
        currentIndex.value = 0
      }
      break
  }

  const nextSong = playlist.value[currentIndex.value]
  if (nextSong) {
    play(nextSong)
  }
}

// 播放上一首
export function prev() {
  if (playlist.value.length === 0) return

  switch (state.value.playMode) {
    case 'random':
      currentIndex.value = Math.floor(Math.random() * playlist.value.length)
      break

    default:
      if (currentIndex.value > 0) {
        currentIndex.value--
      } else if (state.value.playMode === 'loop') {
        currentIndex.value = playlist.value.length - 1
      }
      break
  }

  const prevSong = playlist.value[currentIndex.value]
  if (prevSong) {
    play(prevSong)
  }
}

// 设置播放模式
export function setPlayMode(mode: PlayMode) {
  state.value.playMode = mode
}

// 删除歌曲（同步到后端）
export async function removeFromPlaylist(index: number) {
  if (index >= 0 && index < playlist.value.length) {
    const song = playlist.value[index]
    const wasPlaying = index === currentIndex.value

    // 先从后端删除
    if (song) {
      try {
        await apiRemoveFromPlaylist(song.bvid)
      } catch (e) {
        console.error('Failed to remove song from backend playlist:', e)
      }
    }

    playlist.value.splice(index, 1)
    if (index < currentIndex.value) {
      currentIndex.value--
    } else if (wasPlaying && playlist.value.length > 0) {
      currentIndex.value = Math.min(currentIndex.value, playlist.value.length - 1)
      const nextSong = playlist.value[currentIndex.value]
      if (nextSong) {
        play(nextSong)
      }
    } else if (playlist.value.length === 0) {
      state.value.currentSong = null
      state.value.isPlaying = false
      if (mediaElement) {
        mediaElement.src = ''
      }
    }
  }
}

// 设置当前索引
export function setCurrentIndex(index: number) {
  if (index >= 0 && index < playlist.value.length) {
    currentIndex.value = index
  }
}

// 处理歌曲结束
function handleSongEnd() {
  next()
}

// 播放列表由后端管理，不再本地持久化
// 但当前播放状态仍保存在本地
watch(() => state.value.currentSong, (song) => {
  if (song) {
    storage.set('bilibili-music-current-bvid', song.bvid)
  }
})

// 监听音量变化，自动保存
watch(() => state.value.volume, (newVolume) => {
  storage.set(VOLUME_KEY, newVolume.toString())
})

// 监听播放模式变化，自动保存
watch(() => state.value.playMode, (newMode) => {
  storage.set(MODE_KEY, newMode)
})

// 计算属性
export const currentSong = computed(() => {
  // 优先使用 state 中设置的当前歌曲（用于播放不在播放列表中的歌曲）
  if (state.value.currentSong) {
    return state.value.currentSong
  }
  return playlist.value[currentIndex.value]
})
export const hasNext = computed(() => playlist.value.length > 1)
export const hasPrev = computed(() => playlist.value.length > 1)
export const progress = computed(() => {
  if (state.value.duration === 0) return 0
  return (state.value.currentTime / state.value.duration) * 100
})

// 导出状态
export { state, playlist, currentIndex }
