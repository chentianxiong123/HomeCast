<template>
  <div class="space-y-6">
    <!-- 标题栏 -->
    <div class="flex items-center justify-between">
      <div class="flex items-center space-x-3">
        <div class="w-10 h-10 bg-gradient-to-br from-pink-500 to-violet-500 rounded-xl flex items-center justify-center shadow-lg">
          <n-icon size="20" class="text-white">
            <ListOutline />
          </n-icon>
        </div>
        <div>
          <h1 class="text-xl font-bold text-gray-900 dark:text-white">
            我的播放列表
          </h1>
          <p class="text-sm text-gray-500 dark:text-gray-400">
            共 {{ playlist.length }} 首歌曲
          </p>
        </div>
      </div>
      
      <n-button
        v-if="playlist.length > 0"
        type="error"
        size="small"
        round
        quaternary
        @click="clearPlaylist"
      >
        <template #icon>
          <n-icon><TrashOutline /></n-icon>
        </template>
        清空
      </n-button>
    </div>

    <!-- 播放列表 -->
    <div v-if="playlist.length > 0" class="space-y-2">
      <div
        v-for="(item, index) in playlist"
        :key="item.bvid + index"
        class="group flex items-center space-x-3 p-3 rounded-xl transition-all duration-200"
        :class="{
          'bg-gradient-to-r from-pink-50 to-violet-50 dark:from-pink-900/20 dark:to-violet-900/20 border border-pink-200 dark:border-pink-800': currentIndex === index,
          'bg-white dark:bg-gray-800 hover:bg-gray-50 dark:hover:bg-gray-700/50 border border-gray-100 dark:border-gray-700': currentIndex !== index
        }"
      >
        <!-- 序号/播放状态 -->
        <div class="w-8 flex-shrink-0 text-center">
          <div v-if="currentIndex === index && isPlaying" class="flex justify-center space-x-0.5">
            <div class="w-1 h-4 bg-pink-500 rounded-full animate-pulse" />
            <div class="w-1 h-6 bg-pink-500 rounded-full animate-pulse" style="animation-delay: 0.1s" />
            <div class="w-1 h-3 bg-pink-500 rounded-full animate-pulse" style="animation-delay: 0.2s" />
          </div>
          <span v-else class="text-sm text-gray-400 font-medium">{{ index + 1 }}</span>
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
            v-if="currentIndex !== index"
            class="absolute inset-0 bg-black/30 rounded-lg opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center cursor-pointer"
            @click="playAtIndex(index)"
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
            :class="currentIndex === index ? 'text-pink-600 dark:text-pink-400' : 'text-gray-900 dark:text-white'"
          >
            {{ item.title }}
          </h3>
          <p class="text-sm text-gray-500 dark:text-gray-400 mt-0.5 flex items-center space-x-2">
            <span class="truncate max-w-[100px]">{{ item.artist }}</span>
            <span class="text-gray-300">·</span>
            <span>{{ item.duration }}</span>
          </p>
        </div>

        <!-- 操作 -->
        <div class="flex items-center space-x-1 flex-shrink-0 opacity-0 group-hover:opacity-100 transition-opacity">
          <n-button
            v-if="currentIndex !== index"
            size="small"
            round
            quaternary
            @click="playAtIndex(index)"
          >
            <template #icon>
              <n-icon><PlayOutline /></n-icon>
            </template>
          </n-button>
          <n-button
            v-else
            size="small"
            round
            type="primary"
            quaternary
            @click="togglePlay"
          >
            <template #icon>
              <n-icon>
                <PauseOutline v-if="isPlaying" />
                <PlayOutline v-else />
              </n-icon>
            </template>
          </n-button>
          <n-button
            size="small"
            round
            quaternary
            type="error"
            @click="remove(index)"
          >
            <template #icon>
              <n-icon><CloseOutline /></n-icon>
            </template>
          </n-button>
        </div>
      </div>
    </div>

    <!-- 空状态 -->
    <div v-else class="text-center py-16">
      <div class="w-24 h-24 mx-auto mb-4 bg-gray-100 dark:bg-gray-800 rounded-full flex items-center justify-center">
        <n-icon size="48" class="text-gray-300 dark:text-gray-600">
          <MusicalNotesOutline />
        </n-icon>
      </div>
      <p class="text-gray-500 dark:text-gray-400 text-lg mb-2">播放列表为空</p>
      <p class="text-gray-400 dark:text-gray-500 text-sm mb-6">去搜索一些喜欢的音乐吧</p>
      <n-button 
        type="primary" 
        size="large" 
        round
        @click="$router.push('/search')"
      >
        <template #icon>
          <n-icon><SearchOutline /></n-icon>
        </template>
        去搜索
      </n-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useMessage } from 'naive-ui'
import { usePlayerStore } from '@/stores/player'
import { storeToRefs } from 'pinia'
import { setCurrentIndex, play, loadPlaylistFromBackend } from '@/core/player'
import {
  ListOutline,
  TrashOutline,
  PlayOutline,
  PauseOutline,
  CloseOutline,
  SearchOutline,
  MusicalNotesOutline
} from '@vicons/ionicons5'
import { onMounted } from 'vue'

const message = useMessage()
const { playlist, currentIndex, isPlaying, togglePlay, clearPlaylist: clearPlayerPlaylist, removeFromPlaylist } = usePlayerStore()

// 页面加载时从后端刷新播放列表
onMounted(async () => {
  await loadPlaylistFromBackend()
})

async function playAtIndex(index: number) {
  setCurrentIndex(index)
  const song = playlist.value[index]
  if (song) {
    await play(song)
  }
}

async function remove(index: number) {
  await removeFromPlaylist(index)
  message.success('已从播放列表移除')
}

async function clearPlaylist() {
  await clearPlayerPlaylist()
  message.success('播放列表已清空')
}

function getCoverUrl(cover: string) {
  if (!cover) return ''
  if (cover.startsWith('//')) return 'https:' + cover
  if (cover.startsWith('http')) return cover
  return 'https://i0.hdslb.com' + cover
}
</script>
