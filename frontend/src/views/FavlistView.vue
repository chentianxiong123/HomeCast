<template>
  <div class="space-y-6">
    <!-- 输入收藏夹ID -->
    <div class="max-w-xl mx-auto">
      <div class="bg-white dark:bg-gray-800 rounded-2xl shadow-lg p-6 border border-gray-100 dark:border-gray-700">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white mb-4 flex items-center">
          <n-icon size="22" class="mr-2 text-pink-500">
            <HeartOutline />
          </n-icon>
          我的收藏夹
        </h2>
        
        <div class="space-y-4">
          <div class="flex items-center space-x-4">
            <n-input-group class="flex-1">
              <n-input
                v-model:value="mediaID"
                placeholder="请输入收藏夹ID，如：2320857281"
                size="large"
                class="text-base"
              >
                <template #prefix>
                  <n-icon size="20" class="text-gray-400">
                    <LinkOutline />
                  </n-icon>
                </template>
              </n-input>
            </n-input-group>
            <n-button
              type="primary"
              size="large"
              :loading="loading"
              class="px-6"
              @click="loadFavList"
            >
              加载
            </n-button>
          </div>
          
          <p class="text-sm text-gray-500 dark:text-gray-400">
            💡 提示：在B站收藏夹页面URL中可以找到fid参数，如：
            <code class="bg-gray-100 dark:bg-gray-700 px-1 rounded">space.bilibili.com/xxx/favlist?fid=2320857281</code>
          </p>
        </div>
      </div>
    </div>

    <!-- 收藏夹信息 -->
    <div v-if="favInfo" class="max-w-xl mx-auto">
      <div class="bg-gradient-to-r from-pink-500 to-violet-500 rounded-2xl shadow-lg p-6 text-white">
        <div class="flex items-center space-x-4">
          <img
            :src="getCoverUrl(favInfo.cover)"
            :alt="favInfo.title"
            class="w-20 h-20 rounded-xl object-cover shadow-lg"
            referrerpolicy="no-referrer"
          />
          <div class="flex-1">
            <h3 class="text-xl font-bold">{{ favInfo.title }}</h3>
            <p class="text-white/80 text-sm mt-1 flex items-center space-x-2">
              <n-icon size="16"><PersonOutline /></n-icon>
              <span>{{ favInfo.upper.name }}</span>
            </p>
            <p class="text-white/60 text-sm mt-1">
              共 {{ favInfo.media_count }} 首歌曲
            </p>
          </div>
          <n-button
            type="warning"
            size="large"
            round
            @click="playAll"
          >
            <template #icon>
              <n-icon><PlayOutline /></n-icon>
            </template>
            播放全部
          </n-button>
        </div>
      </div>
    </div>

    <!-- 歌曲列表 -->
    <div v-if="songs.length > 0" class="max-w-xl mx-auto space-y-2">
      <div
        v-for="(item, index) in songs"
        :key="item.bvid"
        class="group flex items-center space-x-3 p-3 bg-white dark:bg-gray-800 rounded-xl border border-gray-100 dark:border-gray-700 hover:shadow-md transition-all duration-200"
      >
        <!-- 序号 -->
        <span class="w-8 text-center text-sm text-gray-400 font-medium">{{ index + 1 }}</span>

        <!-- 封面 -->
        <div class="relative flex-shrink-0">
          <img
            :src="getCoverUrl(item.cover)"
            :alt="item.title"
            class="w-14 h-10 object-cover rounded-lg shadow-sm"
            referrerpolicy="no-referrer"
          />
          <div 
            class="absolute inset-0 bg-black/30 rounded-lg opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center cursor-pointer"
            @click="playSong(item)"
          >
            <n-icon size="16" class="text-white">
              <PlayOutline />
            </n-icon>
          </div>
        </div>

        <!-- 信息 -->
        <div class="flex-1 min-w-0">
          <h4 class="text-base font-medium text-gray-900 dark:text-white line-clamp-1 group-hover:text-pink-500 transition-colors">
            {{ item.title }}
          </h4>
          <p class="text-sm text-gray-500 dark:text-gray-400 flex items-center space-x-2">
            <span>{{ item.artist }}</span>
            <span class="text-gray-300">·</span>
            <span>{{ formatDuration(item.duration) }}</span>
          </p>
        </div>

        <!-- 操作 -->
        <div class="flex-shrink-0 opacity-0 group-hover:opacity-100 transition-opacity">
          <n-button size="small" round quaternary @click="addToPlaylist(item)">
            <template #icon>
              <n-icon><AddOutline /></n-icon>
            </template>
          </n-button>
        </div>
      </div>

      <!-- 加载更多 -->
      <div class="flex justify-center pt-4">
        <n-button
          v-if="hasMore"
          :loading="loadingMore"
          size="large"
          round
          class="px-8"
          @click="loadMore"
        >
          加载更多
        </n-button>
        <p v-else class="text-sm text-gray-400 dark:text-gray-500">
          已加载全部 {{ songs.length }} 首歌曲
        </p>
      </div>
    </div>

    <!-- 空状态 -->
    <div v-else-if="hasSearched && !loading" class="text-center py-16">
      <div class="w-24 h-24 mx-auto mb-4 bg-gray-100 dark:bg-gray-800 rounded-full flex items-center justify-center">
        <n-icon size="48" class="text-gray-300 dark:text-gray-600">
          <HeartOutline />
        </n-icon>
      </div>
      <p class="text-gray-500 dark:text-gray-400 text-lg">请输入收藏夹ID</p>
      <p class="text-gray-400 dark:text-gray-500 text-sm mt-2">输入后在下方显示收藏的音乐</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useMessage } from 'naive-ui'
import { getFavList, type FavListInfo, type FavMedia } from '@/api/favlist'
import { usePlayerStore } from '@/stores/player'
import type { MusicItem } from '@/types'
import {
  HeartOutline,
  PlayOutline,
  AddOutline,
  PersonOutline,
  LinkOutline
} from '@vicons/ionicons5'

const message = useMessage()
const { play: playMusic, addToPlaylist: addSongToPlaylist, clearPlaylist: clearPlayerPlaylist } = usePlayerStore()

const mediaID = ref('')
const loading = ref(false)
const loadingMore = ref(false)
const hasSearched = ref(false)
const favInfo = ref<FavListInfo | null>(null)
const songs = ref<FavMedia[]>([])
const page = ref(1)
const pageSize = 20
const hasMore = ref(false)

async function loadFavList() {
  if (!mediaID.value.trim()) {
    message.warning('请输入收藏夹ID')
    return
  }

  page.value = 1
  songs.value = []
  hasMore.value = false
  favInfo.value = null
  
  loading.value = true
  hasSearched.value = true

  try {
    const result = await getFavList(mediaID.value, page.value, pageSize)
    if (result) {
      favInfo.value = result.info
      songs.value = result.medias
      hasMore.value = result.has_more
    }
  } catch (error) {
    message.error('加载收藏夹失败')
    console.error(error)
  } finally {
    loading.value = false
  }
}

async function loadMore() {
  if (loadingMore.value || !hasMore.value) return

  page.value++
  loadingMore.value = true

  try {
    const result = await getFavList(mediaID.value, page.value, pageSize)
    if (result) {
      songs.value.push(...result.medias)
      hasMore.value = result.has_more
    }
  } catch (error) {
    message.error('加载失败')
    page.value--
  } finally {
    loadingMore.value = false
  }
}

async function playSong(item: FavMedia) {
  const musicItem: MusicItem = {
    bvid: item.bvid,
    title: item.title,
    artist: item.artist,
    cover: item.cover,
    duration: formatDuration(item.duration),
    duration_sec: item.duration,
    play_count: 0
  }
  playMusic(musicItem)
  await addSongToPlaylist(musicItem)
  message.success(`开始播放: ${item.title}`)
}

async function playAll() {
  if (songs.value.length === 0) {
    message.warning('收藏夹为空')
    return
  }

  const musicItems: MusicItem[] = songs.value.map(item => ({
    bvid: item.bvid,
    title: item.title,
    artist: item.artist,
    cover: item.cover,
    duration: formatDuration(item.duration),
    duration_sec: item.duration,
    play_count: 0
  }))

  await clearPlayerPlaylist()
  await addSongToPlaylist(musicItems)
  const firstSong = musicItems[0]
  if (firstSong) {
    playMusic(firstSong)
  }
  message.success(`开始播放全部 ${musicItems.length} 首歌曲`)
}

async function addToPlaylist(item: FavMedia) {
  const musicItem: MusicItem = {
    bvid: item.bvid,
    title: item.title,
    artist: item.artist,
    cover: item.cover,
    duration: formatDuration(item.duration),
    duration_sec: item.duration,
    play_count: 0
  }
  await addSongToPlaylist(musicItem)
  message.success(`已添加到播放列表: ${item.title}`)
}

function formatDuration(seconds: number): string {
  if (!seconds) return '0:00'
  const mins = Math.floor(seconds / 60)
  const secs = seconds % 60
  return `${mins}:${secs.toString().padStart(2, '0')}`
}

function getCoverUrl(cover: string) {
  if (!cover) return ''
  if (cover.startsWith('//')) return 'https:' + cover
  if (cover.startsWith('http')) return cover
  return 'https://i0.hdslb.com' + cover
}
</script>
