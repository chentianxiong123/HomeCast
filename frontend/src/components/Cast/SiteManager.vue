<template>
  <div class="space-y-3">
    <div class="flex items-center justify-between">
      <label class="text-xs font-semibold text-gray-400 uppercase tracking-wider">常用网站</label>
      <n-button size="tiny" quaternary circle @click="showAddModal = true">
        <template #icon><n-icon size="14"><AddOutline /></n-icon></template>
      </n-button>
    </div>

    <!-- 网站列表 -->
    <div class="grid grid-cols-2 gap-2">
      <n-tag
        v-for="site in sites"
        :key="site.url"
        closable
        round
        class="cursor-pointer"
        @click="selectSite(site)"
        @close.stop="removeSite(site.url)"
      >
        {{ site.name }}
      </n-tag>
    </div>

    <!-- 添加网站弹窗 -->
    <n-modal v-model:show="showAddModal" title="添加网站" preset="card" class="w-96">
      <div class="space-y-4">
        <div>
          <label class="text-sm text-gray-500 block mb-1">网站名称</label>
          <n-input v-model:value="newSite.name" placeholder="如：MoMoVOD" />
        </div>
        <div>
          <label class="text-sm text-gray-500 block mb-1">网站URL</label>
          <n-input v-model:value="newSite.url" placeholder="https://momovod.app/vod/xxx.html" />
        </div>
        <div class="flex justify-end gap-2">
          <n-button @click="showAddModal = false">取消</n-button>
          <n-button type="primary" @click="addSite">添加</n-button>
        </div>
      </div>
    </n-modal>

    <!-- 预设网站 -->
    <div v-if="presetSites.length > 0" class="pt-2 border-t border-gray-100">
      <label class="text-xs text-gray-400 block mb-2">推荐网站</label>
      <div class="grid grid-cols-2 gap-2">
        <n-tag
          v-for="site in presetSites"
          :key="site.url"
          round
          type="info"
          class="cursor-pointer"
          @click="selectSite(site)"
        >
          {{ site.name }}
        </n-tag>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { AddOutline } from '@vicons/ionicons5'
import { useMessage } from 'naive-ui'
import { sitesApi } from '@/api'
import type { Site } from '@/api'

const emit = defineEmits<{
  (e: 'select', url: string): void
}>()

const message = useMessage()
const showAddModal = ref(false)
const sites = ref<Site[]>([])
const presetSites = ref<Site[]>([])

const newSite = ref<Site>({ name: '', url: '', site_type: 'detail' })

async function loadSites() {
  const res: any = await sitesApi.getList()
  if (res.code === 0 && res.data) {
    sites.value = res.data.sites || []
    presetSites.value = res.data.preset || []
  }
}

async function addSite() {
  if (!newSite.value.name.trim() || !newSite.value.url.trim()) {
    message.error('请填写完整信息')
    return
  }

  const res: any = await sitesApi.add({
    name: newSite.value.name.trim(),
    url: newSite.value.url.trim(),
    site_type: newSite.value.site_type,
  })

  if (res.code === 0) {
    await loadSites()
    newSite.value = { name: '', url: '', site_type: 'detail' }
    showAddModal.value = false
    message.success('添加成功')
  } else {
    message.error(res.message || '添加失败')
  }
}

async function removeSite(url: string) {
  const res: any = await sitesApi.remove(url)
  if (res.code === 0) {
    await loadSites()
    message.success('删除成功')
  } else {
    message.error(res.message || '删除失败')
  }
}

function selectSite(site: Site) {
  emit('select', site.url)
}

onMounted(() => loadSites())
</script>
