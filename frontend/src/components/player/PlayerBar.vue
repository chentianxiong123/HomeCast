<template>
  <transition name="slide-up">
    <div
      v-if="store.currentSong.value"
      class="fixed bottom-0 left-0 right-0 bg-white/95 dark:bg-gray-800/95 backdrop-blur-lg border-t border-gray-200 dark:border-gray-700 shadow-2xl z-50"
      :class="{ 'speaker-mode': speakerState.isPushing }"
    >
      <!-- 音箱模式指示条 -->
      <div v-if="speakerState.isPushing" class="h-1 bg-gradient-to-r from-blue-500 to-purple-500"></div>

      <!-- 进度条 -->
      <div class="w-full px-4 pt-2">
        <div class="flex items-center space-x-3 text-xs text-gray-500 dark:text-gray-400 mb-1">
          <span class="w-12 text-right font-mono">{{ formatTime(speakerState.isPushing ? speakerState.speakerProgress : store.currentTime.value) }}</span>
          <n-slider
            v-model:value="progressValue"
            :min="0"
            :max="100"
            :step="0.1"
            class="flex-1"
            :disabled="speakerState.isPushing"
            @update:value="handleSeek"
          />
          <span class="w-12 font-mono">{{ formatTime(store.duration.value) }}</span>
        </div>
      </div>

      <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-3">
        <div class="flex items-center justify-between gap-4">
          <!-- 歌曲信息 -->
          <div class="flex items-center space-x-4 flex-1 min-w-0">
            <div class="relative group">
              <img
                :src="getCoverUrl(store.currentSong.value?.cover)"
                :alt="store.currentSong.value?.title"
                class="w-14 h-14 rounded-xl object-cover shadow-md group-hover:shadow-lg transition-shadow"
                referrerpolicy="no-referrer"
              />
              <div v-if="speakerState.isPushing" class="absolute inset-0 bg-blue-500/20 rounded-xl flex items-center justify-center">
                <n-icon size="20" class="text-blue-600">
                  <VolumeHighOutline />
                </n-icon>
              </div>
            </div>
            <div class="min-w-0">
              <p class="text-base font-semibold text-gray-900 dark:text-white truncate">
                {{ store.currentSong.value?.title }}
              </p>
              <p class="text-sm text-gray-500 dark:text-gray-400 truncate">
                {{ speakerState.isPushing ? `正在播放于 ${getDeviceName()}` : store.currentSong.value?.artist }}
              </p>
            </div>
          </div>

          <!-- 控制按钮 -->
          <div class="flex items-center space-x-2 sm:space-x-4">
            <!-- 上一首 (本地模式才显示) -->
            <n-button 
              v-if="!speakerState.isPushing"
              quaternary 
              circle 
              size="large"
              class="hidden sm:flex"
              @click="store.prev"
            >
              <template #icon>
                <n-icon size="28" class="text-gray-600 dark:text-gray-300">
                  <PlaySkipBackOutline />
                </n-icon>
              </template>
            </n-button>

            <!-- 播放/暂停 - 根据模式不同行为 -->
            <n-button
              circle
              :type="speakerState.isPushing ? 'info' : 'primary'"
              size="large"
              :loading="store.isLoading.value && !speakerState.isPushing"
              class="w-14 h-14 sm:w-16 sm:h-16 shadow-lg hover:shadow-xl transition-shadow"
              @click="handlePlayToggle"
            >
              <template #icon>
                <n-icon size="32" :class="speakerState.isPushing ? '' : 'text-white'">
                  <PauseOutline v-if="(speakerState.isPushing && speakerState.speakerStatus === 'playing') || (!speakerState.isPushing && store.isPlaying.value)" />
                  <PlayOutline v-else />
                </n-icon>
              </template>
            </n-button>

            <!-- 下一首 (本地模式才显示) -->
            <n-button 
              v-if="!speakerState.isPushing"
              quaternary 
              circle 
              size="large"
              class="hidden sm:flex"
              @click="store.next"
            >
              <template #icon>
                <n-icon size="28" class="text-gray-600 dark:text-gray-300">
                  <PlaySkipForwardOutline />
                </n-icon>
              </template>
            </n-button>

            <!-- 停止推送按钮 (音箱模式显示) -->
            <n-button
              v-if="speakerState.isPushing"
              quaternary
              circle
              size="large"
              class="text-red-500 hover:text-red-600"
              @click="handleStopPush"
            >
              <template #icon>
                <n-icon size="28">
                  <StopCircleOutline />
                </n-icon>
              </template>
            </n-button>
          </div>

          <!-- 右侧控制 -->
          <div class="flex items-center space-x-2 sm:space-x-4 flex-1 justify-end">
            <!-- 播放模式 (本地模式) -->
            <n-button 
              v-if="!speakerState.isPushing"
              quaternary 
              circle
              size="small"
              class="hidden sm:flex"
              @click="togglePlayMode"
            >
              <template #icon>
                <n-icon size="22" :class="playModeColor">
                  <RepeatOutline v-if="store.playMode.value === 'loop'" />
                  <InfiniteOutline v-else-if="store.playMode.value === 'single'" />
                  <ShuffleOutline v-else-if="store.playMode.value === 'random'" />
                  <ListOutline v-else class="text-pink-500 dark:text-pink-400" />
                </n-icon>
              </template>
            </n-button>

            <!-- 音量控制 -->
            <div class="flex items-center space-x-2 min-w-[100px] sm:min-w-[120px]">
              <n-button quaternary circle size="small" @click="handleToggleMute">
                <template #icon>
                  <n-icon size="24" class="text-gray-500 dark:text-gray-400">
                    <VolumeMuteOutline v-if="(speakerState.isPushing ? speakerVolumeValue === 0 : store.isMuted.value || store.volume.value === 0)" />
                    <VolumeLowOutline v-else-if="(speakerState.isPushing ? speakerVolumeValue < 50 : store.volume.value < 50)" />
                    <VolumeHighOutline v-else />
                  </n-icon>
                </template>
              </n-button>
              <n-slider
                v-model:value="volumeValue"
                :min="0"
                :max="100"
                class="flex-1"
                :tooltip="true"
                @update:value="handleVolumeChange"
              />
            </div>

            <!-- 推送到小爱音箱 / 切换回本地播放 -->
            <SpeakerPush v-if="!speakerState.isPushing && store.currentSong.value" />

            <!-- 返回本地播放按钮 (音箱模式) -->
            <n-button
              v-if="speakerState.isPushing"
              type="tertiary"
              size="small"
              @click="handleStopPush"
            >
              <template #icon>
                <n-icon><DesktopOutline /></n-icon>
              </template>
              本地播放
            </n-button>
          </div>
        </div>
      </div>
    </div>
  </transition>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { usePlayerStore } from '@/stores/player'
import { speakerState, stopPush, speakerTogglePlay, speakerSetVolume } from '@/stores/player'
import SpeakerPush from '@/components/Player/SpeakerPush.vue'
import {
  PlayOutline,
  PauseOutline,
  PlaySkipBackOutline,
  PlaySkipForwardOutline,
  VolumeHighOutline,
  VolumeLowOutline,
  VolumeMuteOutline,
  RepeatOutline,
  ShuffleOutline,
  ListOutline,
  ExpandOutline,
  InfiniteOutline,
  StopCircleOutline,
  DesktopOutline
} from '@vicons/ionicons5'
import type { PlayMode } from '@/types'

const store = usePlayerStore()

const progressValue = ref(0)
const volumeValue = ref(store.volume.value)
const speakerVolumeValue = ref(speakerState.speakerVolume)

// 监听进度变化
watch(() => store.progress.value, (val) => {
  if (!speakerState.isPushing) {
    progressValue.value = val
  }
})

// 监听音量变化
watch(() => store.volume.value, (val) => {
  if (!speakerState.isPushing) {
    volumeValue.value = val
  }
})

// 监听音箱音量变化
watch(() => speakerState.speakerVolume, (val) => {
  if (speakerState.isPushing) {
    speakerVolumeValue.value = val
  }
})

watch(volumeValue, (val) => {
  if (!speakerState.isPushing) {
    store.setVolume(val)
  }
})

const playModeColor = computed(() => {
  return store.playMode.value !== 'order'
    ? 'text-pink-500 dark:text-pink-400'
    : 'text-gray-500 dark:text-gray-400'
})

function handleSeek(val: number) {
  if (!speakerState.isPushing) {
    store.seekToPercent(val)
  }
}

function handlePlayToggle() {
  if (speakerState.isPushing) {
    // 音箱模式：控制音箱播放/暂停
    speakerTogglePlay()
  } else {
    // 本地模式：控制本地播放
    store.togglePlay()
  }
}

function handleVolumeChange(val: number) {
  if (speakerState.isPushing) {
    // 音箱模式：设置音箱音量
    speakerSetVolume(val)
  }
}

async function handleStopPush() {
  await stopPush()
}

function handleToggleMute() {
  if (speakerState.isPushing) {
    if (speakerVolumeValue.value === 0) {
      speakerVolumeValue.value = 80
      speakerSetVolume(80)
    } else {
      speakerVolumeValue.value = 0
      speakerSetVolume(0)
    }
  } else {
    if (volumeValue.value === 0) {
      volumeValue.value = 80
    } else {
      volumeValue.value = 0
    }
    store.toggleMute()
  }
}

function togglePlayMode() {
  const modes: PlayMode[] = ['order', 'loop', 'single', 'random']
  const currentIdx = modes.indexOf(store.playMode.value)
  const nextIdx = (currentIdx + 1) % modes.length
  const nextMode = modes[nextIdx]
  if (nextMode) {
    store.setPlayMode(nextMode)
  }
}

function getDeviceName(): string {
  const device = speakerState.speakerDevices.find(d => d.did === speakerState.pushTargetDid)
  return device?.name || '小爱音箱'
}

function formatTime(seconds: number): string {
  if (!seconds || isNaN(seconds)) return '0:00'
  const mins = Math.floor(seconds / 60)
  const secs = Math.floor(seconds % 60)
  return `${mins}:${secs.toString().padStart(2, '0')}`
}

function getCoverUrl(cover: string | undefined) {
  if (!cover) return ''
  if (cover.startsWith('//')) return 'https:' + cover
  if (cover.startsWith('http')) return cover
  return 'https://i0.hdslb.com' + cover
}
</script>

<style scoped>
.slide-up-enter-active,
.slide-up-leave-active {
  transition: transform 0.3s ease, opacity 0.3s ease;
}

.slide-up-enter-from,
.slide-up-leave-to {
  transform: translateY(100%);
  opacity: 0;
}

.speaker-mode {
  border-top: 3px solid transparent;
  border-image: linear-gradient(to right, #3b82f6, #8b5cf6) 1;
}
</style>
