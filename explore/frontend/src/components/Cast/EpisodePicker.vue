<template>
  <div class="space-y-3">
    <div class="flex items-center justify-between">
      <span class="text-sm text-gray-500 dark:text-gray-400">
        共 {{ totalEpisodes }} 集
      </span>
      <n-pagination
        v-if="totalPages > 1"
        v-model:page="currentPage"
        :page-count="totalPages"
        :page-size="pageSize"
        size="small"
      />
    </div>

    <div class="overflow-hidden">
      <div class="grid grid-cols-10 gap-2">
        <n-button
          v-for="ep in pagedEpisodes"
          :key="ep.url"
          size="small"
          :type="selectedIndex === ep.index ? 'primary' : 'default'"
          round
          @click="handleClick(ep)"
          @dblclick="$emit('play', ep)"
        >
          {{ ep.index }}
        </n-button>
      </div>
    </div>

    <!-- 选中的集数详情和链接管理 -->
    <div v-if="selectedEpisode" class="bg-gray-50 dark:bg-gray-800 rounded-lg p-3 space-y-2">
      <div class="text-sm font-medium">
        第{{ selectedEpisode.index }}集 - {{ selectedEpisode.title }}
      </div>
      
      <!-- 链接管理 -->
      <div class="flex items-center gap-2">
        <n-input
          :value="selectedEpisode.url"
          readonly
          size="small"
          class="flex-1 text-xs"
        />
        <n-button size="small" @click="copyLink">
          <template #icon>
            <n-icon><CopyOutline /></n-icon>
          </template>
          复制链接
        </n-button>
        <n-button size="small" type="primary" @click="$emit('play', selectedEpisode)">
          <template #icon>
            <n-icon><PlayCircleOutline /></n-icon>
          </template>
          播放
        </n-button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { CopyOutline, PlayCircleOutline } from '@vicons/ionicons5'
import { useMessage } from 'naive-ui'
import type { Episode } from '@/api'

const props = withDefaults(defineProps<{
  episodes: Episode[]
  pageSize?: number
}>(), {
  pageSize: 50,  // 每页显示50集
})

const emit = defineEmits<{
  (e: 'select', episode: Episode): void
  (e: 'play', episode: Episode): void
}>()

const message = useMessage()
const currentPage = ref(1)
const selectedIndex = ref<number | null>(null)
const selectedEpisode = computed(() =>
  props.episodes.find(ep => ep.index === selectedIndex.value) || null,
)

const totalEpisodes = computed(() => props.episodes.length)
const totalPages = computed(() => Math.ceil(props.episodes.length / props.pageSize))

const pagedEpisodes = computed(() => {
  const start = (currentPage.value - 1) * props.pageSize
  return props.episodes.slice(start, start + props.pageSize)
})

function handleClick(episode: Episode) {
  selectedIndex.value = episode.index
  emit('select', episode)
}

async function copyLink() {
  if (!selectedEpisode.value?.url) return
  try {
    await navigator.clipboard.writeText(selectedEpisode.value.url)
    message.success('链接已复制到剪贴板')
  } catch {
    message.error('复制失败')
  }
}

function formatDuration(seconds?: number): string {
  if (!seconds) return '--:--'
  const m = Math.floor(seconds / 60)
  const s = Math.floor(seconds % 60)
  return `${m}:${s.toString().padStart(2, '0')}`
}
</script>
