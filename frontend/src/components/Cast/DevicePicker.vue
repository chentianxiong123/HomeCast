<template>
  <div class="space-y-3">
    <div class="flex items-center justify-between">
      <span class="text-sm font-medium text-gray-700">
        DLNA 设备 ({{ devices.length }})
      </span>
      <n-button size="small" quaternary @click="$emit('refresh')">
        <template #icon>
          <n-icon size="16"><RefreshOutline /></n-icon>
        </template>
        刷新
      </n-button>
    </div>

    <div v-if="devices.length === 0" class="text-center py-8 text-gray-400">
      未发现 DLNA 设备，请确保设备在同一局域网
    </div>

    <div v-else class="grid grid-cols-1 sm:grid-cols-2 gap-3">
      <n-card
        v-for="device in devices"
        :key="device.udn"
        hoverable
        :class="[
          'cursor-pointer transition-all',
          modelValue === device.udn ? 'ring-2 ring-blue-500' : ''
        ]"
        @click="$emit('update:modelValue', device.udn)"
      >
        <div class="flex items-center gap-3">
          <div class="min-w-0 flex-1">
            <p class="font-medium text-gray-900 truncate">{{ device.name }}</p>
            <p class="text-xs text-gray-500">{{ device.ip }}</p>
          </div>
          <n-icon v-if="modelValue === device.udn" size="20" class="text-blue-500">
            <CheckmarkCircleOutline />
          </n-icon>
        </div>
      </n-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { RefreshOutline, CheckmarkCircleOutline } from '@vicons/ionicons5'
import type { DLNADevice } from '@/api'

defineProps<{
  devices: DLNADevice[]
  modelValue: string | null
}>()

defineEmits<{
  (e: 'update:modelValue', udn: string): void
  (e: 'refresh'): void
}>()
</script>