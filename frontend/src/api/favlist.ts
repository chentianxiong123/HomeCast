import {
  MOCK_FAVLISTS,
  MOCK_FAV_INFO,
  MOCK_FAV_MEDIA,
} from '../mock/data'
import type { FavListInfo, FavMedia, FavListResult } from './favlist'

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
