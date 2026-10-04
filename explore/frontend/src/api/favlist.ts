// 收藏夹 API（对齐桌面音乐播放器那套：本地收藏，去重置顶 + ☆/★ toggle）
export interface FavSong {
  bvid: string
  title: string
  artist: string
  cover: string
  duration: string
  duration_sec: number
  play_count?: number
}

export const favApi = {
  async getFavs(): Promise<FavSong[]> {
    const res = await import('./request').then(m => m.default.get('/fav/list'))
    return (res.data.data as FavSong[]) || []
  },
  async addFav(song: FavSong): Promise<void> {
    await import('./request').then(m => m.default.post('/fav/add', song))
  },
  async removeFav(bvid: string): Promise<void> {
    await import('./request').then(m => m.default.delete(`/fav/${bvid}`))
  },
  async clearFavs(): Promise<void> {
    await import('./request').then(m => m.default.post('/fav/clear'))
  },
}
