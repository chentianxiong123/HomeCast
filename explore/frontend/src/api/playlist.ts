import request from './request'
import type { MusicItem } from '@/types'

export interface PlaylistResult {
  list: MusicItem[]
}

export async function getPlaylist(): Promise<MusicItem[]> {
  const res = await request.get('/playlist')
  return res.data.data.list || []
}

export async function addToPlaylist(bvid: string): Promise<MusicItem | null> {
  const res = await request.post(`/playlist/add/${bvid}`)
  if (res.data.code === 0) {
    return res.data.data
  }
  return null
}

export async function removeFromPlaylist(bvid: string): Promise<boolean> {
  const res = await request.post(`/playlist/remove/${bvid}`)
  return res.data.code === 0
}

export async function clearPlaylist(): Promise<boolean> {
  const res = await request.post('/playlist/clear')
  return res.data.code === 0
}
