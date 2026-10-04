<template>
  <div class="space-y-6">
    <!-- 标题栏 -->
    <div class="flex items-center justify-between">
      <div class="flex items-center space-x-3">
        <div class="w-10 h-10 bg-gradient-to-br from-amber-400 to-pink-500 rounded-xl flex items-center justify-center shadow-lg">
          <n-icon size="20" class="text-white">
            <Star />
          </n-icon>
        </div>
        <div>
          <h1 class="text-xl font-bold text-gray-900 dark:text-white">
            我的收藏
          </h1>
          <p class="text-sm text-gray-500 dark:text-gray-400">
            共 {{ favs.length }} 首歌曲
          </p>
        </div>
      </div>

      <n-button
        v-if="favs.length > 0"
        type="error"
        size="small"
        round
        quaternary
        :loading="clearing"
        @click="handleClear"
      >
        <template #icon>
          <n-icon><TrashOutline /></n-icon>
        </template>
        清空
      </n-button>
    </div>

    <!-- 收藏列表（对齐桌面：去重置顶，新的在最前） -->
    <div v-if="favs.length > 0" class="space-y-2">
      <div
        v-for="(item, index) in favs"
        :key="item.bvid"
        class="group flex items-center space-x-3 p-3 rounded-xl transition-all duration-200"
        :class="currentSong?.bvid === item.bvid
          ? 'bg-gradient-to-r from-amber-50 to-pink-50 dark:from-amber-900/20 dark:to-pink-900/20 border border-amber-200 dark:border-amber-800'
          : 'bg-white dark:bg-gray-800 hover:bg-gray-50 dark:hover:bg-gray-700/50 border border-gray-100 dark:border-gray-700'"
      >
        <!-- 序号 -->
        <div class="w-8 flex-shrink-0 text-center">
          <span class="text-sm text-gray-400 font-medium">{{ index + 1 }}</span>
        </div>

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
            @click="playFav(index)"
          >
            <n-icon size="16" class="text-white">
              <PlayOutline />
            </n-icon>
          </div>
        </div>

        <!-- 信息 -->
        <div class="flex-1 min-w-0">
          <h3
            class="text-base font-medium line-clamp-1"
            :class="currentSong?.bvid === item.bvid ? 'text-pink-600 dark:text-pink-400' : 'text-gray-900 dark:text-white'"
          >
            {{ item.title }}
          </h3>
          <p class="text-sm text-gray-500 dark:text-gray-400 mt-0.5 flex items-center space-x-2">
            <span class="truncate max-w-[100px]">{{ item.artist || '未知作者' }}</span>
            <span class="text-gray-300">·</span>
            <span>{{ item.duration || fmtSec(item.duration_sec) }}</span>
          </p>
        </div>

        <!-- 操作 -->
        <div class="flex items-center space-x-1 flex-shrink-0 opacity-0 group-hover:opacity-100 transition-opacity">
          <n-button
            size="small"
            round
            type="primary"
            quaternary
            @click="playFav(index)"
          >
            <template #icon>
              <n-icon>
                <PauseOutline v-if="currentSong?.bvid === item.bvid && isPlaying" />
                <PlayOutline v-else />
              </n-icon>
            </template>
          </n-button>
          <n-button
            size="small"
            round
            quaternary
            type="warning"
            @click="unfav(item.bvid)"
          >
            <template #icon>
              <n-icon><Star /></n-icon>
            </template>
          </n-button>
        </div>
      </div>
    </div>

    <!-- 空状态 -->
    <div v-else class="text-center py-20">
      <div class="w-24 h-24 mx-auto mb-4 bg-gray-100 dark:bg-gray-800 rounded-full flex items-center justify-center">
        <n-icon size="48" class="text-gray-300 dark:text-gray-600">
          <StarOutline />
        </n-icon>
      </div>
      <h2 class="text-lg font-semibold text-gray-700 dark:text-gray-300">
        还没有收藏的歌曲
      </h2>
      <p class="text-sm text-gray-500 dark:text-gray-400 mt-2">
        去搜索页点 ☆ 收藏，喜欢的歌都会存在这里
      </p>
      <n-button
        type="primary"
        round
        class="mt-5"
        @click="$router.push('/search')"
      >
        去搜索
      </n-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useMessage } from 'naive-ui'
import { useFavStore } from '@/stores/favs'
import { usePlayerStore } from '@/stores/player'
import { Star, StarOutline, PlayOutline, PauseOutline, TrashOutline } from '@vicons/ionicons5'

const message = useMessage()
const favStore = useFavStore()
const playerStore = usePlayerStore()
const clearing = ref(false)

const favs = favStore.favs
const currentSong = playerStore.currentSong
const isPlaying = playerStore.isPlaying

onMounted(() => {
  favStore.loadFavs()
})

function getCoverUrl(cover: string | undefined) {
  if (!cover) return ''
  if (cover.startsWith('//')) return 'https:' + cover
  if (cover.startsWith('http')) return cover
  return 'https://i0.hdslb.com' + cover
}

function fmtSec(sec: number) {
  if (!sec) return ''
  const m = Math.floor(sec / 60)
  const s = sec % 60
  return `${m}:${s.toString().padStart(2, '0')}`
}

// 播放收藏（对齐桌面：直接播放该曲）
async function playFav(index: number) {
  const item = favs.value[index]
  if (!item) return
  await playerStore.play({
    bvid: item.bvid,
    title: item.title,
    artist: item.artist,
    cover: item.cover,
    duration: item.duration || fmtSec(item.duration_sec),
    duration_sec: item.duration_sec,
    play_count: item.play_count || 0,
  })
}

async function unfav(bvid: string) {
  await favStore.removeFav(bvid)
  message.success('已取消收藏')
}

async function handleClear() {
  clearing.value = true
  await favStore.clearFavs()
  clearing.value = false
  message.success('已清空收藏')
}
</script>
