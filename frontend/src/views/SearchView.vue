<template>
  <div class="space-y-6">
    <!-- 搜索框 -->
    <div class="max-w-2xl mx-auto">
      <div class="relative">
        <n-input-group class="shadow-lg rounded-full overflow-hidden">
          <n-input
            v-model:value="keyword"
            placeholder="搜索喜欢的音乐..."
            size="large"
            class="text-base"
            @keydown.enter="handleSearch"
          >
            <template #prefix>
              <n-icon size="20" class="text-gray-400">
                <SearchOutline />
              </n-icon>
            </template>
          </n-input>
          <n-button
            type="primary"
            size="large"
            :loading="loading"
            class="px-8"
            @click="handleSearch"
          >
            搜索
          </n-button>
        </n-input-group>
      </div>
    </div>

    <!-- 搜索结果 -->
    <div v-if="results.length > 0" class="space-y-4">
      <div class="flex items-center justify-between">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
          搜索结果
        </h2>
        <span class="text-sm text-gray-500 dark:text-gray-400">
          已加载 {{ results.length }} 首
        </span>
      </div>

      <div class="grid gap-3">
        <div
          v-for="item in results"
          :key="item.bvid"
          class="group flex items-center space-x-4 p-4 bg-white dark:bg-gray-800 rounded-2xl shadow-sm hover:shadow-md transition-all duration-200 border border-gray-100 dark:border-gray-700"
        >
          <!-- 封面 -->
          <div class="relative flex-shrink-0">
            <img
              :src="getCoverUrl(item.cover)"
              :alt="item.title"
              class="w-20 h-14 sm:w-24 sm:h-16 object-cover rounded-xl shadow-sm"
              referrerpolicy="no-referrer"
            />
            <div class="absolute inset-0 bg-black/30 rounded-xl opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center cursor-pointer"
                 @click="playMusic(item)">
              <n-icon size="24" class="text-white">
                <PlayOutline />
              </n-icon>
            </div>
          </div>

          <!-- 信息 -->
          <div class="flex-1 min-w-0">
            <h3 class="text-base sm:text-lg font-semibold text-gray-900 dark:text-white line-clamp-1 group-hover:text-pink-500 transition-colors">
              {{ item.title }}
            </h3>
            <div class="flex items-center space-x-3 mt-1 text-sm text-gray-500 dark:text-gray-400">
              <span class="flex items-center space-x-1">
                <n-icon size="14"><PersonOutline /></n-icon>
                <span class="truncate max-w-[120px]">{{ item.artist }}</span>
              </span>
              <span class="flex items-center space-x-1">
                <n-icon size="14"><TimeOutline /></n-icon>
                <span>{{ item.duration }}</span>
              </span>
            </div>
          </div>

          <!-- 操作按钮 -->
          <div class="flex items-center space-x-2 flex-shrink-0">
            <n-button
              type="primary"
              size="small"
              round
              class="shadow-sm"
              @click="playMusic(item)"
            >
              <template #icon>
                <n-icon><PlayOutline /></n-icon>
              </template>
              播放
            </n-button>
            <n-button
              size="small"
              round
              quaternary
              class="hidden sm:flex"
              @click="addToPlaylist(item)"
            >
              <template #icon>
                <n-icon><AddOutline /></n-icon>
              </template>
              添加
            </n-button>
          </div>
        </div>
      </div>

      <!-- 加载更多 -->
      <div class="flex justify-center pt-6">
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
          没有更多结果了
        </p>
      </div>
    </div>

    <!-- 空状态 -->
    <div v-else-if="hasSearched && !loading" class="text-center py-16">
      <div class="w-24 h-24 mx-auto mb-4 bg-gray-100 dark:bg-gray-800 rounded-full flex items-center justify-center">
        <n-icon size="48" class="text-gray-300 dark:text-gray-600">
          <SearchOutline />
        </n-icon>
      </div>
      <p class="text-gray-500 dark:text-gray-400 text-lg">未找到相关音乐</p>
      <p class="text-gray-400 dark:text-gray-500 text-sm mt-2">换个关键词试试</p>
    </div>

    <!-- 初始状态 -->
    <div v-else-if="!hasSearched" class="text-center py-16">
      <div class="w-24 h-24 mx-auto mb-4 bg-gradient-to-br from-pink-100 to-violet-100 dark:from-pink-900/30 dark:to-violet-900/30 rounded-full flex items-center justify-center">
        <n-icon size="48" class="text-pink-400">
          <MusicalNotesOutline />
        </n-icon>
      </div>
      <p class="text-gray-500 dark:text-gray-400 text-lg">输入关键词开始搜索</p>
      <p class="text-gray-400 dark:text-gray-500 text-sm mt-2">发现哔哩哔哩上的精彩音乐</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useMessage } from 'naive-ui'
import { searchMusic } from '@/api/music'
import { usePlayerStore } from '@/stores/player'
import type { MusicItem } from '@/types'
import {
  SearchOutline,
  PlayOutline,
  AddOutline,
  PersonOutline,
  TimeOutline,
  MusicalNotesOutline
} from '@vicons/ionicons5'

const message = useMessage()
const { play: playSong, addToPlaylist: addSongToPlaylist } = usePlayerStore()

const keyword = ref('')
const loading = ref(false)
const loadingMore = ref(false)
const hasSearched = ref(false)
const results = ref<MusicItem[]>([])
const page = ref(1)
const pageSize = 20
const hasMore = ref(true)

async function handleSearch() {
  if (!keyword.value.trim()) {
    message.warning('请输入搜索关键词')
    return
  }

  page.value = 1
  results.value = []
  hasMore.value = true
  
  loading.value = true
  hasSearched.value = true

  try {
    const res = await searchMusic(keyword.value, page.value, pageSize)
    results.value = res.list
    hasMore.value = res.list.length >= pageSize
  } catch (error) {
    message.error('搜索失败')
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
    const res = await searchMusic(keyword.value, page.value, pageSize)
    results.value.push(...res.list)
    hasMore.value = res.list.length >= pageSize
  } catch (error) {
    message.error('加载失败')
    console.error(error)
    page.value--
  } finally {
    loadingMore.value = false
  }
}

async function playMusic(item: MusicItem) {
  await playSong(item)
  await addSongToPlaylist(item)
  message.success(`开始播放: ${item.title}`)
}

async function addToPlaylist(item: MusicItem) {
  await addSongToPlaylist(item)
  message.success(`已添加到播放列表: ${item.title}`)
}

function getCoverUrl(cover: string) {
  if (!cover) return ''
  if (cover.startsWith('//')) return 'https:' + cover
  if (cover.startsWith('http')) return cover
  return 'https://i0.hdslb.com' + cover
}
</script>
