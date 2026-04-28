<template>
  <div class="p-4 sm:p-6 lg:p-8 max-w-6xl mx-auto">
    <div class="mb-6">
      <h1 class="text-2xl font-bold text-gray-900 dark:text-white mb-2">
        缓存管理
      </h1>
      <p class="text-gray-500 dark:text-gray-400">
        管理已缓存的音频文件，删除不需要的缓存以释放空间
      </p>
    </div>

    <!-- 统计卡片 -->
    <div class="grid grid-cols-1 sm:grid-cols-3 gap-4 mb-6">
      <n-card>
        <div class="text-center">
          <div class="text-3xl font-bold text-blue-600">{{ summary.count }}</div>
          <div class="text-sm text-gray-500 mt-1">缓存数量</div>
        </div>
      </n-card>
      <n-card>
        <div class="text-center">
          <div class="text-3xl font-bold text-green-600">{{ summary.total_mb }} MB</div>
          <div class="text-sm text-gray-500 mt-1">已用空间</div>
        </div>
      </n-card>
      <n-card>
        <div class="text-center">
          <div class="text-3xl font-bold text-purple-600">{{ summary.max_mb }} MB</div>
          <div class="text-sm text-gray-500 mt-1">最大容量</div>
        </div>
      </n-card>
    </div>

    <!-- 操作按钮 -->
    <div class="flex justify-between items-center mb-4">
      <n-space>
        <n-button type="primary" @click="loadCacheList" :loading="loading">
          <template #icon>
            <n-icon><RefreshOutline /></n-icon>
          </template>
          刷新列表
        </n-button>
        <n-button type="error" @click.stop="handleClearAll" :loading="clearing">
          <template #icon>
            <n-icon><TrashOutline /></n-icon>
          </template>
          清空全部
        </n-button>
      </n-space>
      <n-input
        v-model:value="searchText"
        placeholder="搜索 BVID..."
        clearable
        style="width: 200px"
      />
    </div>

    <!-- 缓存列表 -->
    <n-data-table
      :columns="columns"
      :data="filteredItems"
      :loading="loading"
      :pagination="pagination"
      striped
    />

    <!-- 空状态 -->
    <n-empty v-if="!loading && filteredItems.length === 0" description="暂无缓存文件" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, h } from 'vue'
import { useMessage, NButton, NIcon, NSpace } from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import {
  RefreshOutline,
  TrashOutline,
  PlayOutline,
} from '@vicons/ionicons5'
import { cacheApi } from '../api'

const message = useMessage()
const loading = ref(false)
const clearing = ref(false)
const searchText = ref('')
const items = ref<any[]>([])
const summary = ref({
  count: 0,
  total_mb: 0,
  max_mb: 500,
})

const pagination = {
  pageSize: 10,
}

const filteredItems = computed(() => {
  if (!searchText.value) return items.value
  return items.value.filter(item => 
    item.bvid.toLowerCase().includes(searchText.value.toLowerCase())
  )
})

const columns: DataTableColumns<any> = [
  {
    title: 'BVID',
    key: 'bvid',
    width: 150,
  },
  {
    title: '文件大小',
    key: 'size_mb',
    width: 120,
    render(row) {
      return h('span', {}, `${row.size_mb} MB`)
    },
  },
  {
    title: '码率',
    key: 'bitrate',
    width: 100,
  },
  {
    title: '缓存时间',
    key: 'cached_at_str',
    width: 180,
  },
  {
    title: '操作',
    key: 'actions',
    width: 200,
    render(row) {
      return h(NSpace, {}, {
        default: () => [
          h(NButton, {
            size: 'small',
            type: 'primary',
            onClick: () => playCache(row.bvid),
          }, {
            default: () => '播放',
            icon: () => h(NIcon, null, { default: () => h(PlayOutline) }),
          }),
          h(NButton, {
            size: 'small',
            type: 'error',
            onClick: () => deleteCache(row.bvid),
          }, {
            default: () => '删除',
            icon: () => h(NIcon, null, { default: () => h(TrashOutline) }),
          }),
        ],
      })
    },
  },
]

async function loadCacheList() {
  loading.value = true
  try {
    const res: any = await cacheApi.getList()
    if (res.code === 0) {
      items.value = res.data?.items || []
      summary.value = res.data?.summary || { count: 0, total_mb: 0, max_mb: 500 }
    } else {
      message.error(res.message || '加载失败')
    }
  } catch (e) {
    message.error('加载缓存列表失败')
  } finally {
    loading.value = false
  }
}

async function deleteCache(bvid: string) {
  try {
    const res: any = await cacheApi.delete(bvid)
    if (res.code === 0) {
      message.success(res.message)
      await loadCacheList()
    } else {
      message.error(res.message || '删除失败')
    }
  } catch (e) {
    message.error('删除失败')
  }
}

function handleClearAll() {
  console.log('清空按钮被点击')
  const result = window.confirm('确定要清空所有缓存吗？此操作不可恢复。')
  console.log('对话框结果:', result)
  if (result === false) {
    console.log('用户取消')
    return
  }

  console.log('开始清空...')
  clearing.value = true
  cacheApi.clearAll()
    .then((res: any) => {
      if (res.code === 0) {
        message.success(res.message)
        loadCacheList()
      } else {
        message.error(res.message || '清空失败')
      }
    })
    .catch(() => {
      message.error('清空失败')
    })
    .finally(() => {
      clearing.value = false
    })
}

function playCache(bvid: string) {
  // 跳转到音乐页面播放
  window.open(`/#/music?bvid=${bvid}`, '_blank')
}

onMounted(() => {
  loadCacheList()
})
</script>
