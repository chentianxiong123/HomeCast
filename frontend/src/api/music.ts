import { MOCK_SONGS } from '../mock/data'
import type { MusicItem, VideoInfo, AudioStream, SearchResult } from '@/types'

const USE_MOCK = false

export async function searchMusic(keyword: string, page = 1, pageSize = 20): Promise<SearchResult> {
  if (USE_MOCK) {
    await delay(600)
    const start = (page - 1) * pageSize
    return {
      total: MOCK_SONGS.length,
      list: MOCK_SONGS.slice(start, start + pageSize).map(s => ({
        ...s,
        title: keyword ? `${s.title} (${keyword}相关)` : s.title,
      })),
    }
  }

  const res = await import('./request').then(m => m.default.get('/music/search', { params: { keyword, page, page_size: pageSize } }))
  return res.data.data
}

export async function getVideoInfo(bvid: string): Promise<VideoInfo> {
  if (USE_MOCK) {
    await delay(300)
    const song = MOCK_SONGS.find(s => s.bvid === bvid) || MOCK_SONGS[0]
    return {
      bvid: song.bvid,
      aid: 12345678,
      cid: 87654321,
      title: song.title,
      desc: '这是一首很好听的歌',
      cover: `https://picsum.photos/seed/${bvid}/400/225`,
      duration: song.duration_sec,
      artist: song.artist,
      artist_id: 12345,
    }
  }

  const res = await import('./request').then(m => m.default.get(`/music/info/${bvid}`))
  return res.data.data
}

export async function getAudioStream(bvid: string, quality = 64): Promise<AudioStream> {
  if (USE_MOCK) {
    await delay(200)
    return {
      url: `https://example.com/audio/${bvid}/quality_${quality}.m4a`,
      quality,
      size: 5242880,
      mime_type: 'audio/m4a',
      codecs: 'mp4a.40.2',
      cached: false,
    }
  }

  // 调用 /audio/{bvid} 获取音频信息（包括是否有缓存）
  const res = await import('./request').then(m => m.default.get(`/music/audio/${bvid}`, { params: { quality } }))
  return res.data.data
}

export function getAudioProxyUrl(bvid: string, quality = 192): string {
  const baseUrl = import.meta.env.VITE_API_BASE_URL || '/api/v1'
  return `${baseUrl}/proxy/audio/${bvid}?quality=${quality}`
}

function delay(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms))
}
