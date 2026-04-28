<template>
  <n-config-provider :theme="theme">
    <n-message-provider>
      <div class="min-h-screen bg-gradient-to-br from-gray-50 to-gray-100 dark:from-gray-900 dark:to-gray-800">
        <!-- 顶部导航 -->
        <header class="bg-white/80 dark:bg-gray-800/80 backdrop-blur-md shadow-sm sticky top-0 z-40">
          <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
            <div class="flex items-center justify-between h-16">
              <div class="flex items-center space-x-4">
                <!-- Logo -->
                <div class="flex items-center space-x-2">
                  <div class="w-10 h-10 bg-gradient-to-br from-pink-500 to-violet-500 rounded-xl flex items-center justify-center shadow-lg">
                    <svg class="w-6 h-6 text-white" viewBox="0 0 24 24" fill="currentColor">
                      <path d="M9 18V6H7v12h2zm8-12v12h2V6h-2z"/>
                    </svg>
                  </div>
                  <h1 class="text-xl font-bold bg-gradient-to-r from-pink-500 to-violet-500 bg-clip-text text-transparent hidden sm:block">
                    哔哩音乐
                  </h1>
                </div>
              </div>
              
              <!-- 导航 -->
              <nav class="flex space-x-2">
                <router-link
                  v-for="item in navItems"
                  :key="item.path"
                  :to="item.path"
                  class="flex items-center space-x-1 px-4 py-2 rounded-full text-base font-medium transition-all duration-200"
                  :class="$route.path === item.path 
                    ? 'bg-gradient-to-r from-pink-500 to-violet-500 text-white shadow-md' 
                    : 'text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'"
                >
                  <n-icon :size="20">
                    <component :is="item.icon" />
                  </n-icon>
                  <span>{{ item.label }}</span>
                </router-link>
              </nav>
            </div>
          </div>
        </header>

        <!-- 主内容区 -->
        <main class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6 pb-36">
          <router-view v-slot="{ Component }">
            <transition name="fade" mode="out-in">
              <component :is="Component" />
            </transition>
          </router-view>
        </main>

        <!-- 播放器控制栏 -->
        <PlayerBar />
      </div>
    </n-message-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useOsTheme, darkTheme } from 'naive-ui'
import { MusicalNotesOutline, SearchOutline, HeartOutline, TvOutline, SaveOutline } from '@vicons/ionicons5'
import PlayerBar from '@/components/player/PlayerBar.vue'
import { initPlayer } from '@/core/player'

const osThemeRef = useOsTheme()
const theme = computed(() => (osThemeRef.value === 'dark' ? darkTheme : null))

const navItems = [
  { path: '/music', label: '音乐', icon: MusicalNotesOutline },
  { path: '/search', label: '搜索', icon: SearchOutline },
  { path: '/favlist', label: '收藏夹', icon: HeartOutline },
  { path: '/cast', label: '投屏', icon: TvOutline },
  { path: '/cache', label: '缓存', icon: SaveOutline },
]

// 初始化播放器（加载本地存储的数据）
onMounted(() => {
  initPlayer()
})
</script>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
