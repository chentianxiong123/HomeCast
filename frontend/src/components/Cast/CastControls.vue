<template>
  <div v-if="isCasting" class="mt-4 rounded-xl overflow-hidden border border-blue-200 bg-gradient-to-b from-blue-50/80 to-white">
    <div class="px-5 pt-4 pb-3">
      <div class="flex items-center gap-3 mb-4">
        <div class="w-10 h-10 rounded-full bg-blue-100 flex items-center justify-center shrink-0">
          <n-icon size="20" class="text-blue-600"><TvOutline /></n-icon>
        </div>
        <div class="min-w-0 flex-1">
          <p class="font-semibold text-gray-900 text-sm truncate">{{ episodeTitle }}</p>
          <p class="text-xs text-gray-500 truncate">{{ deviceName }}</p>
        </div>
        <n-tag :type="stateTagType" size="small" round :bordered="false">
          {{ stateLabel }}
        </n-tag>
      </div>

      <div class="relative group cursor-pointer py-2" @click="handleProgressClick">
        <div class="h-2 bg-gray-200 rounded-full overflow-hidden">
          <div
            class="h-full rounded-full transition-all duration-300 relative"
            :class="progressBarClass"
            :style="{ width: progressPercent + '%' }"
          >
            <div class="absolute right-0 top-1/2 -translate-y-1/2 w-3.5 h-3.5 bg-white rounded-full shadow-md border-2 opacity-0 group-hover:opacity-100 transition-opacity"></div>
          </div>
        </div>
        <div class="flex justify-between mt-1.5">
          <span class="text-[11px] font-mono text-blue-600 font-medium">{{ displayPosition }}</span>
          <span class="text-[11px] font-mono text-gray-400">{{ displayDuration }}</span>
        </div>
      </div>

      <div class="flex items-center justify-center gap-6 mt-2 mb-1">
        <button
          class="w-11 h-11 flex items-center justify-center rounded-full hover:bg-gray-100 transition-colors"
          @click="$emit('control', 'stop')"
          title="停止"
        >
          <n-icon size="22" class="text-gray-500"><StopIcon /></n-icon>
        </button>

        <button
          class="w-14 h-14 flex items-center justify-center rounded-full transition-all shadow-md"
          :class="transportState === 'PLAYING' ? 'bg-amber-400 hover:bg-amber-500 shadow-amber-200' : 'bg-blue-500 hover:bg-blue-600 shadow-blue-200'"
          @click="$emit('control', transportState === 'PLAYING' ? 'pause' : 'play')"
        >
          <n-icon size="28" class="text-white">
            <PauseOutline v-if="transportState === 'PLAYING'" />
            <PlayOutline v-else />
          </n-icon>
        </button>

        <div class="flex items-center gap-1.5 ml-2">
          <button class="w-8 h-8 flex items-center justify-center rounded-full hover:bg-gray-100 transition-colors" @click="volumeDown" title="音量-">
            <n-icon size="16" class="text-gray-400"><RemoveOutline /></n-icon>
          </button>
          <div class="w-16 relative group/vol">
            <div class="h-1.5 bg-gray-200 rounded-full overflow-hidden">
              <div class="h-full bg-blue-400 rounded-full transition-all" :style="{ width: volume + '%' }"></div>
            </div>
          </div>
          <button class="w-8 h-8 flex items-center justify-center rounded-full hover:bg-gray-100 transition-colors" @click="volumeUp" title="音量+">
            <n-icon size="16" class="text-gray-400"><AddOutline /></n-icon>
          </button>
          <span class="text-[11px] text-gray-400 w-7 text-right">{{ volume }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted } from 'vue'
import {
  TvOutline,
  PlayOutline,
  PauseOutline,
  StopCircleOutline as StopIcon,
  AddOutline,
  RemoveOutline,
} from '@vicons/ionicons5'

const props = defineProps<{
  isCasting: boolean
  deviceUdn: string
  episodeTitle: string
  deviceName: string
}>()

const emit = defineEmits<{
  (e: 'control', action: string): void
}>()

const transportState = ref('STOPPED')
const positionSec = ref(0)
const durationSec = ref(0)
const volume = ref(80)
let pollTimer: ReturnType<typeof setInterval> | null = null

const progressPercent = computed(() => {
  if (!durationSec.value || durationSec.value <= 0) return 0
  return Math.min((positionSec.value / durationSec.value) * 100, 100)
})

const displayPosition = computed(() => formatTime(positionSec.value))
const displayDuration = computed(() => formatTime(durationSec.value))

const stateLabel = computed(() => {
  const map: Record<string, string> = {
    PLAYING: '播放中',
    PAUSED_PLAYBACK: '已暂停',
    PAUSED: '已暂停',
    STOPPED: '已停止',
    TRANSITIONING: '加载中',
    NO_MEDIA_PRESENT: '无媒体',
  }
  return map[transportState.value] || transportState.value || '未知'
})

const stateTagType = computed<'success' | 'warning' | 'default' | 'info'>(() => {
  if (transportState.value === 'PLAYING') return 'success'
  if (transportState.value === 'PAUSED' || transportState.value === 'PAUSED_PLAYBACK') return 'warning'
  if (transportState.value === 'TRANSITIONING') return 'info'
  return 'default'
})

const progressBarClass = computed(() => {
  if (transportState.value === 'PLAYING') return 'bg-blue-500'
  if (transportState.value === 'PAUSED') return 'bg-amber-400'
  return 'bg-gray-400'
})

function formatTime(totalSec: number): string {
  if (!totalSec || totalSec <= 0) return '0:00'
  const m = Math.floor(totalSec / 60)
  const s = Math.floor(totalSec % 60)
  return `${m}:${s.toString().padStart(2, '0')}`
}

function handleProgressClick(e: MouseEvent) {
  const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
  const pct = ((e.clientX - rect.left) / rect.width) * 100
  if (!durationSec.value || durationSec.value <= 0) return
  const targetSec = Math.floor((pct / 100) * durationSec.value)
  emit('control', `seek:${formatTime(targetSec)}`)
}

function volumeUp() {
  if (volume.value < 100) {
    volume.value = Math.min(volume.value + 10, 100)
    emit('control', `volume:${volume.value}`)
  }
}

function volumeDown() {
  if (volume.value > 0) {
    volume.value = Math.max(volume.value - 10, 0)
    emit('control', `volume:${volume.value}`)
  }
}

async function fetchStatus() {
  if (!props.deviceUdn) return
  try {
    const { castApi } = await import('@/api')
    const res: any = await castApi.getStatus(props.deviceUdn)
    if (res.code === 0 && res.data) {
      transportState.value = res.data.transport?.state || 'STOPPED'
      const posStr = res.data.position?.rel_time || '0:00:00'
      const durStr = res.data.position?.duration || '0:00:00'
      positionSec.value = parseTime(posStr)
      durationSec.value = parseTime(durStr)
    }
  } catch {}
}

function parseTime(t: string): number {
  if (!t || t === '0:00:00') return 0
  const parts = t.split(':').map(Number).filter(Boolean)
  if (parts.length === 3) return parts[0] * 3600 + parts[1] * 60 + parts[2]
  if (parts.length === 2) return parts[0] * 60 + parts[1]
  return parseInt(t) || 0
}

onMounted(() => {
  if (props.isCasting) {
    fetchStatus()
    pollTimer = setInterval(fetchStatus, 3000)
  }
})

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
})
</script>