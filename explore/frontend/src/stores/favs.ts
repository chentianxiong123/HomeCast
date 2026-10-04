// 收藏夹共享状态（对齐桌面：☆/★ toggle，各页实时同步）
import { reactive, computed } from 'vue'
import { favApi, type FavSong } from '@/api/favlist'
import type { MusicItem } from '@/types'

const state = reactive<{ favs: FavSong[]; loaded: boolean }>({
  favs: [],
  loaded: false,
})

export function useFavStore() {
  async function loadFavs(force = false) {
    if (state.loaded && !force) return
    try {
      state.favs = await favApi.getFavs()
    } catch (e) {
      console.warn('收藏加载失败:', e)
    }
    state.loaded = true
  }

  function isFav(bvid: string) {
    return state.favs.some(f => f.bvid === bvid)
  }

  async function addFav(song: MusicItem | FavSong) {
    const entry: FavSong = {
      bvid: song.bvid,
      title: song.title,
      artist: song.artist || '',
      cover: song.cover || '',
      duration: song.duration || '',
      duration_sec: song.duration_sec || 0,
      play_count: (song as FavSong).play_count || 0,
    }
    state.favs = [entry, ...state.favs.filter(f => f.bvid !== entry.bvid)]
    try { await favApi.addFav(entry) } catch (e) { console.warn('收藏失败:', e) }
  }

  async function removeFav(bvid: string) {
    state.favs = state.favs.filter(f => f.bvid !== bvid)
    try { await favApi.removeFav(bvid) } catch (e) { console.warn('取消收藏失败:', e) }
  }

  async function toggleFav(song: MusicItem | FavSong) {
    if (isFav(song.bvid)) await removeFav(song.bvid)
    else await addFav(song)
  }

  async function clearFavs() {
    state.favs = []
    try { await favApi.clearFavs() } catch (e) { console.warn('清空收藏失败:', e) }
  }

  return {
    favs: computed(() => state.favs),
    loaded: computed(() => state.loaded),
    loadFavs,
    isFav,
    addFav,
    removeFav,
    toggleFav,
    clearFavs,
  }
}
