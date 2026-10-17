<template>
  <teleport to="body">
    <transition name="fade">
      <div v-if="open" class="fixed inset-0 z-[70] bg-black/90 backdrop-blur-sm" @click.self="close">
        <!-- 顶部：歌名 + 关闭（z-10 确保不被歌词滚动区盖住） -->
        <div
          class="absolute top-0 inset-x-0 z-10 flex items-center justify-between px-4 pt-4"
          style="padding-top: max(1rem, env(safe-area-inset-top))"
        >
          <div class="min-w-0 flex-1">
            <p class="text-white text-base font-semibold truncate">{{ store.currentSong.value?.title }}</p>
            <p v-if="store.lyricMeta.value?.name" class="text-white/50 text-xs truncate mt-0.5">
              {{ store.lyricMeta.value.name }}{{ store.lyricMeta.value.artist ? ` — ${store.lyricMeta.value.artist}` : '' }}
            </p>
          </div>
          <button class="w-9 h-9 rounded-full bg-white/10 text-white flex items-center justify-center active:bg-white/20"
                  @click="close">
            <n-icon size="20"><CloseOutline /></n-icon>
          </button>
        </div>

        <!-- 歌词滚动区（KTV：当前行居中高亮） -->
        <div
          v-if="store.lyrics.value.length > 0"
          ref="scrollRef"
          class="absolute inset-0 overflow-y-auto px-6 py-[38vh] scroll-smooth"
          @click.self="close"
        >
          <p
            v-for="(l, i) in store.lyrics.value"
            :key="i"
            class="text-center leading-relaxed transition-all duration-300"
            :class="i === curIdx
              ? 'text-white text-[26px] sm:text-3xl font-semibold py-4'
              : i === curIdx - 1 || i === curIdx + 1
                ? 'text-white/50 text-lg py-2.5'
                : 'text-white/25 text-base py-2'"
          >
            {{ l[1] }}
          </p>
        </div>

        <!-- 无歌词提示 -->
        <div v-else-if="store.lyricLoading.value" class="absolute inset-0 flex items-center justify-center">
          <div class="flex items-center space-x-2 text-white/50">
            <n-icon size="18" class="animate-spin"><RefreshOutline /></n-icon>
            <span class="text-sm">加载歌词中...</span>
          </div>
        </div>
        <div v-else class="absolute inset-0 flex flex-col items-center justify-center text-white/40 px-10 text-center">
          <n-icon size="44"><MusicalNotesOutline /></n-icon>
          <p class="text-sm mt-3">未找到这首的歌词</p>
        </div>

        <!-- 底部进度提示 -->
        <div class="absolute bottom-0 inset-x-0 flex justify-center pb-6"
             style="padding-bottom: max(1.5rem, env(safe-area-inset-bottom))">
          <span class="text-white/40 text-xs font-mono">{{ fmtTime(store.currentTime.value) }} / {{ fmtTime(store.duration.value) }}</span>
        </div>
      </div>
    </transition>
  </teleport>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { CloseOutline, RefreshOutline, MusicalNotesOutline } from '@vicons/ionicons5'
import { usePlayerStore } from '@/stores/player'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ (e: 'update:open', v: boolean): void }>()

const store = usePlayerStore()
const scrollRef = ref<HTMLElement | null>(null)

const curIdx = computed(() => {
  const lines = store.lyrics.value
  const t = store.currentTime.value
  if (!lines.length) return -1
  // 当前时间之前最后一行
  let idx = -1
  for (let i = 0; i < lines.length; i++) {
    const l = lines[i]
    if (l && l[0] <= t) idx = i
    else break
  }
  // 无词前奏时对准第一行
  return idx < 0 ? 0 : idx
})

function fmtTime(t: number) {
  if (!t || t < 0) return '0:00'
  const m = Math.floor(t / 60)
  const s = Math.floor(t % 60)
  return `${m}:${s.toString().padStart(2, '0')}`
}

function close() {
  emit('update:open', false)
}

// 当前行变化 → 滚动居中（只滚一次，防抖动）
watch(curIdx, (idx) => {
  if (!props.open || idx < 0) return
  const el = scrollRef.value
  if (!el) return
  const child = el.children[idx] as HTMLElement | undefined
  if (child) {
    child.scrollIntoView({ block: 'center', behavior: 'smooth' })
  }
})

// 打开面板时滚到当前行（不带动画）
watch(() => props.open, (o) => {
  if (o) {
    const idx = curIdx.value
    if (idx >= 0) {
      requestAnimationFrame(() => {
        const el = scrollRef.value
        if (!el) return
        const child = el.children[idx] as HTMLElement | undefined
        if (child) child.scrollIntoView({ block: 'center' })
      })
    }
  }
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
