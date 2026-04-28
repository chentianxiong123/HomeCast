<template>
  <div class="flex gap-2">
    <div class="flex-1">
      <n-input
        v-model:value="url"
        placeholder="粘贴视频链接..."
        size="large"
        clearable
        :disabled="loading"
        @keyup.enter="handleSniff"
      >
        <template #prefix>
          <n-icon class="text-gray-400"><LinkOutline /></n-icon>
        </template>
      </n-input>
    </div>
    <n-button
      type="primary"
      size="large"
      :loading="loading"
      strong
      :disabled="loading"
      @click="handleSniff"
    >
      <template #icon><n-icon><SearchOutline /></n-icon></template>
      {{ loading ? '解析中...' : '嗅探' }}
    </n-button>
  </div>
  <div v-if="loading" class="mt-2 text-sm text-gray-400">
    正在解析集数列表，请稍候（约8-10秒）...
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { LinkOutline, SearchOutline } from '@vicons/ionicons5'
import { castApi } from '@/api'
import type { SniffResult } from '@/api'

const emit = defineEmits<{
  (e: 'sniff', result: SniffResult): void
}>()

const url = ref('')
const loading = ref(false)

function setUrl(newUrl: string) {
  url.value = newUrl
}

defineExpose({ setUrl, handleSniff })

async function handleSniff() {
  if (!url.value.trim()) return
  loading.value = true
  console.log('[Sniffer] Starting sniff for:', url.value.trim())
  try {
    const res: any = await castApi.sniff(url.value.trim())
    console.log('[Sniffer] API response:', res)
    if (res.code === 0 && res.data) {
      console.log('[Sniffer] Success, emitting result:', res.data)
      emit('sniff', res.data)
    } else {
      console.error('[Sniffer] API error:', res.message || 'Unknown error')
      window.$message?.error?.(res.message || '嗅探失败')
    }
  } catch (e: any) {
    console.error('[Sniffer] Request failed:', e)
    window.$message?.error?.(e.message || '请求失败')
  } finally { loading.value = false }
}
</script>