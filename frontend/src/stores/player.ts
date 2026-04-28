import { reactive, computed } from 'vue'
import { speakerApi } from '../api'
import {
  state,
  playlist,
  currentIndex,
  currentSong,
  progress,
  play,
  togglePlay,
  pause,
  seekTo,
  seekToPercent,
  setVolume,
  toggleMute,
  addToPlaylist,
  clearPlaylist,
  next,
  prev,
  setPlayMode,
  removeFromPlaylist,
  setCurrentIndex,
} from '../core/player'
import type { SpeakerDevice, DLNADevice, Episode, MusicItem } from '../api'

export type { CurrentSong, CastState }

export const speakerState = reactive({
  pushTargetDid: '' as string | null,
  isPushing: false,
  speakerDevices: [] as SpeakerDevice[],
  dlnaDevices: [] as DLNADevice[],
  castState: null as any | null,
  isLoggedIn: false,
  isLoading: false,
  // 音箱控制模式
  speakerVolume: 50 as number,
  speakerStatus: 'idle' as string, // idle, playing, paused
  speakerProgress: 0 as number,
  speakerDuration: 0 as number,
})

export function usePlayerStore() {
  return {
    playerStore: state,
    playlist,
    currentIndex,
    currentSong,
    progress: computed(() => {
      if (state.value.duration === 0) return 0
      return (state.value.currentTime / state.value.duration) * 100
    }),
    isPlaying: computed(() => state.value.isPlaying),
    isLoading: computed(() => state.value.isLoading),
    currentTime: computed(() => state.value.currentTime),
    duration: computed(() => state.value.duration),
    volume: computed(() => state.value.volume),
    isMuted: computed(() => state.value.isMuted),
    playMode: computed(() => state.value.playMode),
    play,
    togglePlay,
    pause,
    seekTo,
    seekToPercent,
    setVolume,
    toggleMute,
    addToPlaylist,
    clearPlaylist,
    next,
    prev,
    setPlayMode,
    removeFromPlaylist,
    setCurrentIndex,
  }
}

export async function loadSpeakerStatus() {
  try {
    const res: any = await speakerApi.getStatus()
    if (res.code === 0) {
      speakerState.isLoggedIn = res.data?.is_logged_in || false
      return res.data
    }
  } catch (e) {
    console.error('Failed to load speaker status:', e)
  }
  return null
}

export async function loadSpeakerDevices() {
  try {
    // 先检查登录状态
    const status = await loadSpeakerStatus()
    if (!status?.is_logged_in) {
      speakerState.speakerDevices = []
      return
    }

    const res: any = await speakerApi.getDevices()
    if (res.code === 0) {
      speakerState.speakerDevices = res.data || []
    }
  } catch (e) {
    console.error('Failed to load speaker devices:', e)
  }
}

export async function loginSpeaker(account: string, password: string, cookie: string = '') {
  speakerState.isLoading = true
  try {
    const res: any = await speakerApi.login(account, password, cookie)
    if (res.code === 0) {
      speakerState.isLoggedIn = true
      // 登录成功后加载设备
      await loadSpeakerDevices()
      return { success: true, message: res.message }
    } else {
      return { success: false, message: res.message }
    }
  } catch (e: any) {
    return { success: false, message: e.message || '登录失败' }
  } finally {
    speakerState.isLoading = false
  }
}

// ========== 二维码登录 ==========
export async function generateQRCode() {
  speakerState.isLoading = true
  try {
    const res: any = await speakerApi.generateQRCode()
    if (res.code === 0 && res.data?.is_logged_in) {
      speakerState.isLoggedIn = true
      await loadSpeakerDevices()
    }
    return res
  } catch (e: any) {
    return { code: -1, message: e.message || '生成二维码失败', data: null }
  } finally {
    speakerState.isLoading = false
  }
}

export async function checkQRStatus() {
  try {
    const res: any = await speakerApi.checkQRStatus()
    if (res.code === 0 && res.data?.status === 'success') {
      speakerState.isLoggedIn = true
      await loadSpeakerDevices()
    }
    return res
  } catch (e: any) {
    return { code: -1, message: e.message || '检查状态失败', data: { status: 'failed' } }
  }
}

export async function resetQRLogin() {
  await speakerApi.resetQRLogin()
}

export async function logoutSpeaker() {
  try {
    await speakerApi.logout()
    speakerState.isLoggedIn = false
    speakerState.speakerDevices = []
    speakerState.pushTargetDid = null
    speakerState.isPushing = false
  } catch (e) {
    console.error('Failed to logout:', e)
  }
}

export async function pushToSpeaker(deviceDid: string) {
  try {
    speakerState.isPushing = true
    speakerState.pushTargetDid = deviceDid

    const res: any = await speakerApi.play(
      deviceDid,
      currentSong.value?.bvid || '',
      currentSong.value?.title || '',
    )

    if (res.code === 0) {
      speakerState.isPushing = true
      // 推送成功后开始轮询音箱状态
      startSpeakerPolling()
      return true
    }
    return false
  } catch (e) {
    console.error('Push to speaker failed:', e)
    return false
  }
}

export async function stopPush() {
  if (!speakerState.pushTargetDid) return
  try {
    await speakerApi.control(speakerState.pushTargetDid, 'stop')
    speakerState.pushTargetDid = null
    speakerState.isPushing = false
    // 停止轮询
    stopSpeakerPolling()
  } catch (e) {
    console.error('Stop push failed:', e)
  }
}

// ========== 音箱控制模式 ==========

let _speakerPollTimer: number | null = null

function startSpeakerPolling() {
  stopSpeakerPolling()
  // 每 3 秒查询一次音箱状态
  _speakerPollTimer = window.setInterval(async () => {
    await fetchSpeakerStatus()
  }, 3000)
  // 立即查询一次
  fetchSpeakerStatus()
}

function stopSpeakerPolling() {
  if (_speakerPollTimer) {
    clearInterval(_speakerPollTimer)
    _speakerPollTimer = null
  }
}

async function fetchSpeakerStatus() {
  if (!speakerState.pushTargetDid) return

  try {
    const res: any = await speakerApi.getPlayerStatus(speakerState.pushTargetDid)
    if (res.code === 0 && res.data) {
      const status = res.data.status
      speakerState.speakerStatus = status === 1 ? 'playing' : (status === 2 ? 'paused' : 'idle')
      speakerState.speakerVolume = res.data.volume || 50
      // 音箱没有返回进度信息，使用本地进度模拟
    }

    // 获取音量
    const volRes: any = await speakerApi.getVolume(speakerState.pushTargetDid)
    if (volRes.code === 0 && volRes.data?.volume !== undefined) {
      speakerState.speakerVolume = volRes.data.volume
    }
  } catch (e) {
    console.warn('Fetch speaker status failed:', e)
  }
}

export async function speakerTogglePlay() {
  if (!speakerState.pushTargetDid) return

  try {
    if (speakerState.speakerStatus === 'playing') {
      await speakerApi.control(speakerState.pushTargetDid, 'pause')
      speakerState.speakerStatus = 'paused'
    } else {
      await speakerApi.control(speakerState.pushTargetDid, 'resume')
      speakerState.speakerStatus = 'playing'
    }
  } catch (e) {
    console.error('Speaker toggle play failed:', e)
  }
}

export async function speakerSetVolume(volume: number) {
  if (!speakerState.pushTargetDid) return

  try {
    volume = Math.max(0, Math.min(100, volume))
    await speakerApi.setVolume(speakerState.pushTargetDid, volume)
    speakerState.speakerVolume = volume
  } catch (e) {
    console.error('Set speaker volume failed:', e)
  }
}
