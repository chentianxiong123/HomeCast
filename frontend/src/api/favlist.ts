// 收藏夹类型定义（原代码自引用 './favlist' 导致类型缺失，补全于此）
export interface FavMedia {
  id: number
  bvid: string
  title: string
  cover: string
  duration: number
  artist: string
}

export interface FavListInfo {
  id: number
  title: string
  cover: string
  media_count: number
  intro: string
  upper?: { mid: number; name: string; face: string }
}

export interface FavListResult {
  info: FavListInfo
  medias: FavMedia[]
  has_more: boolean
}

import {
  MOCK_FAVLISTS,
  MOCK_FAV_INFO,
  MOCK_FAV_MEDIA,
} from '../mock/data'

const USE_MOCK = false

export async function getFavList(mediaID: string | number, page = 1, pageSize = 20): Promise<FavListResult | null> {
  if (USE_MOCK) {
    await delay(500)
    const start = (page - 1) * pageSize
    return {
      info: MOCK_FAV_INFO as unknown as FavListInfo,
      medias: MOCK_FAV_MEDIA.slice(start, start + pageSize),
      has_more: start + pageSize < MOCK_FAV_MEDIA.length,
    }
  }

  try {
    const res = await import('./request').then(m =>
      m.default.get(`/favlist/${mediaID}`, { params: { page, page_size: pageSize } })
    )
    return res.data.data.data
  } catch {
    return null
  }
}

export async function getFavListInfo(mediaID: string | number): Promise<FavListInfo | null> {
  if (USE_MOCK) {
    await delay(300)
    return MOCK_FAV_INFO as unknown as FavListInfo
  }

  try {
    const res = await import('./request').then(m => m.default.get(`/favlist/info/${mediaID}`))
    return res.data.data.data
  } catch {
    return null
  }
}

export async function getFavlistLists(): Promise<Array<{ mid: number; name: string; count: number }>> {
  if (USE_MOCK) {
    await delay(200)
    return MOCK_FAVLISTS
  }

  try {
    const res = await import('./request').then(m => m.default.get('/favlist/list'))
    return res.data.data.data || []
  } catch {
    return []
  }
}

function delay(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms))
}
