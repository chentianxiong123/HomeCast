<template>
  <div class="p-6 max-w-7xl mx-auto">
    <div class="space-y-8">
      <div class="flex items-center gap-4">
        <div class="w-12 h-12 rounded-xl bg-gradient-to-br from-blue-500 to-purple-600 flex items-center justify-center shadow-lg shadow-blue-500/20">
          <n-icon size="28" color="#fff"><TvOutline /></n-icon>
        </div>
        <div>
          <h1 class="text-2xl font-bold tracking-tight">视频投屏</h1>
          <p class="text-gray-500 text-sm mt-0.5">输入任意视频链接，嗅探后投屏到 DLNA 设备</p>
        </div>
      </div>

      <!-- 第一行：视频链接 + 网站管理 + DLNA设备 -->
      <div class="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div>
          <label class="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-2 block">视频链接</label>
          <UrlSniffer ref="urlSnifferRef" @sniff="handleSniff" />
        </div>
        <div class="bg-white dark:bg-gray-800 rounded-xl p-4 shadow-sm border border-gray-100">
          <SiteManager @select="handleSiteSelect" />
        </div>
        <div class="bg-white dark:bg-gray-800 rounded-xl p-4 shadow-sm border border-gray-100">
          <div class="flex items-center justify-between mb-2">
            <label class="text-xs font-semibold text-gray-400 uppercase tracking-wider">DLNA 设备</label>
            <n-button size="tiny" quaternary circle @click="loadDevices">
              <template #icon><n-icon size="14"><RefreshOutline /></n-icon></template>
            </n-button>
          </div>
          <DevicePicker v-model="selectedUdn" :devices="dlnaDevices" @refresh="loadDevices" />
        </div>
      </div>

      <!-- 第二行：集数列表（全宽） -->
      <div v-if="sniffResult" class="bg-white dark:bg-gray-800 rounded-xl p-6 shadow-sm border border-gray-100">
        <div class="flex items-center justify-between mb-4">
          <div class="flex items-center gap-3">
            <span class="text-lg font-semibold">{{ sniffResult.title || '视频详情' }}</span>
            <n-tag :type="sniffMethodType" size="small" :bordered="false" round>{{ sniffResult.sniff_method }}</n-tag>
            <span class="text-gray-400 text-sm">{{ (sniffResult.episodes_list || sniffResult.episodes).length }} 集</span>
          </div>
        </div>
        <EpisodePicker
          v-if="sniffResult.episodes_list?.length"
          :episodes="sniffResult.episodes_list"
          @select="handleEpisodeSelect"
          @play="handleEpisodePlay"
        />
        <EpisodePicker
          v-else-if="sniffResult.episodes?.length"
          :episodes="sniffResult.episodes"
          @play="handlePlay"
        />
        <div v-else-if="sniffResult.episodes_list?.length === 0" class="text-gray-400">
          正在加载集数列表...
        </div>
      </div>

      <!-- 视频播放器 -->
      <div v-if="videoUrl || isHls" class="bg-black rounded-xl overflow-hidden shadow-lg">
        <video
          ref="videoRef"
          :src="videoUrl || undefined"
          controls
          autoplay
          class="w-full aspect-video"
        />
      </div>

      <!-- 第三行：投屏控制 -->
      <div class="bg-white dark:bg-gray-800 rounded-xl p-4 shadow-sm border border-gray-100">
        <CastControls
          v-if="isCasting"
          :is-casting="isCasting"
          :device-udn="selectedUdn || ''"
          :episode-title="currentEpisode?.title || ''"
          :device-name="currentDevice?.name || ''"
          @control="handleControl"
        />
        <div v-else-if="selectedEpisode && selectedUdn" class="text-center py-4">
          <p class="text-sm text-gray-500 mb-2">准备投屏到 {{ currentDevice?.name }}</p>
          <p class="font-medium mb-3">{{ selectedEpisode.title }}</p>
          <n-button type="primary" strong @click="doCast">
            开始投屏
          </n-button>
        </div>
        <div v-else class="text-center py-4 text-gray-400">
          选择设备和集数后即可投屏
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, nextTick } from 'vue'
import { TvOutline, RefreshOutline } from '@vicons/ionicons5'
import { castApi } from '@/api'
import type { SniffResult, Episode, DLNADevice } from '@/api'
import UrlSniffer from '@/components/Cast/UrlSniffer.vue'
import EpisodePicker from '@/components/Cast/EpisodePicker.vue'
import DevicePicker from '@/components/Cast/DevicePicker.vue'
import CastControls from '@/components/Cast/CastControls.vue'
import SiteManager from '@/components/Cast/SiteManager.vue'
import Hls from 'hls.js'

const sniffResult = ref<SniffResult | null>(null)
const dlnaDevices = ref<DLNADevice[]>([])
const selectedUdn = ref<string | null>(null)
const selectedEpisode = ref<Episode | null>(null)
const isCasting = ref(false)
const urlSnifferRef = ref<InstanceType<typeof UrlSniffer> | null>(null)
const videoUrl = ref<string | null>(null)
const videoRef = ref<HTMLVideoElement | null>(null)
const hlsInstance = ref<Hls | null>(null)
const isHls = ref(false)

function handleSiteSelect(url: string) {
  // 设置URL到输入框并自动嗅探
  if (urlSnifferRef.value) {
    urlSnifferRef.value.setUrl(url)
    urlSnifferRef.value.handleSniff()
  }
}

const currentDevice = computed(() =>
  dlnaDevices.value.find((d) => d.udn === selectedUdn.value) || null,
)

const currentEpisode = computed(() => selectedEpisode.value)

const sniffMethodType = computed(() => {
  const method = sniffResult.value?.sniff_method || ''
  if (method === 'yt-dlp') return 'success'
  if (method === 'bilibili-api') return 'info'
  return 'warning'
})

async function loadDevices() {
  try {
    console.log('Loading DLNA devices...')
    const res: any = await castApi.getDevices()
    console.log('DLNA devices response:', res)
    if (res.code === 0) {
      dlnaDevices.value = res.data || []
      console.log('DLNA devices loaded:', dlnaDevices.value.length)
    } else {
      console.error('Failed to load devices:', res.message)
    }
  } catch (e) {
    console.error('Load devices error:', e)
  }
}

function handleSniff(result: SniffResult) {
  sniffResult.value = result
  isCasting.value = false
  if (result.episodes?.length) {
    selectedEpisode.value = result.episodes[0] || null
  } else if (result.episodes_list?.length) {
    selectedEpisode.value = result.episodes_list[0] || null
  }
  loadDevices()
}

async function handleEpisodeSelect(episode: Episode) {
  selectedEpisode.value = episode
  console.log('Selected episode:', episode.url)
}

async function handleEpisodePlay(episode: Episode) {
  console.log('Fetching video URL for:', episode.url)
  selectedEpisode.value = episode
  try {
    // 先嗅探播放页获取视频URL
    const sniffRes: any = await castApi.sniff(episode.url)
    if (sniffRes.code !== 0 || !sniffRes.data?.episodes?.length) {
      console.error('Failed to sniff video:', sniffRes.message)
      return
    }
    const videoUrlFromSniff = sniffRes.data.episodes[0].url
    console.log('Sniffed video URL:', videoUrlFromSniff)

    // 获取可播放的流URL
    const playRes: any = await castApi.playUrl(videoUrlFromSniff, episode.title)
    if (playRes.code === 0 && playRes.data?.url) {
      const streamUrl = playRes.data.url
      const isHlsStream = playRes.data.hls || streamUrl.includes('.m3u8')
      isHls.value = isHlsStream
      console.log('Playing stream:', streamUrl, 'hls:', isHlsStream)

      // 清理之前的 HLS 实例
      if (hlsInstance.value) {
        hlsInstance.value.destroy()
        hlsInstance.value = null
      }

      if (isHlsStream && Hls.isSupported()) {
        // 使用 HLS.js 播放
        // 先设置 isHls 触发 video 元素渲染，等待下一帧再初始化 HLS
        await nextTick()
        await new Promise(resolve => setTimeout(resolve, 100))

        const hls = new Hls({
          enableWorker: true,
        })
        hls.loadSource(streamUrl)
        hls.attachMedia(videoRef.value!)
        hls.on(Hls.Events.MANIFEST_PARSED, () => {
          videoRef.value?.play()
        })
        hls.on(Hls.Events.ERROR, (event: any, data: any) => {
          console.error('HLS error:', data)
        })
        hlsInstance.value = hls
        videoUrl.value = null  // 不需要 src
      } else {
        // 直接使用 video 标签播放
        videoUrl.value = streamUrl
      }
    } else {
      console.error('Failed to get play URL:', playRes.message)
    }
  } catch (e) {
    console.error('Failed to fetch video URL:', e)
  }
}

async function handlePlay(episode: Episode) {
  selectedEpisode.value = episode
  // 投屏到DLNA设备
  if (!selectedUdn.value) {
    console.warn('No DLNA device selected')
    return
  }
  try {
    // 先嗅探获取视频URL
    const sniffRes: any = await castApi.sniff(episode.url)
    if (sniffRes.code !== 0 || !sniffRes.data?.episodes?.length) {
      console.error('Failed to sniff video:', sniffRes.message)
      return
    }
    const videoUrlFromSniff = sniffRes.data.episodes[0].url
    console.log('Casting video URL:', videoUrlFromSniff)

    // 获取播放URL
    const playRes: any = await castApi.playUrl(videoUrlFromSniff, episode.title)
    if (playRes.code !== 0 || !playRes.data?.url) {
      console.error('Failed to get play URL:', playRes.message)
      return
    }

    // 投屏
    const castUrl = playRes.data.url
    const res: any = await castApi.start(castUrl, selectedUdn.value, episode.title)
    if (res.code === 0) {
      isCasting.value = true
      console.log('Casting started successfully')
    } else {
      console.error('Cast failed:', res.message)
    }
  } catch (e) {
    console.error('Cast error:', e)
  }
}

async function doCast() {
  console.log('doCast called', { episode: selectedEpisode.value, udn: selectedUdn.value })
  if (!selectedEpisode.value) {
    console.warn('No episode selected')
    return
  }
  if (!selectedUdn.value) {
    console.warn('No DLNA device selected')
    return
  }
  try {
    // 如果 url 是播放页 URL，先获取视频 URL
    let castUrl = selectedEpisode.value.url
    console.log('Casting URL:', castUrl)
    if (castUrl.includes('/play/')) {
      console.log('Sniffing play page...')
      const sniffRes: any = await castApi.sniff(castUrl)
      if (sniffRes.code !== 0 || !sniffRes.data?.episodes?.length) {
        console.error('Failed to sniff video:', sniffRes.message)
        return
      }
      castUrl = sniffRes.data.episodes[0].url
      console.log('Sniffed video URL:', castUrl)
    }
    console.log('Calling cast API...')
    const res: any = await castApi.start(castUrl, selectedUdn.value, selectedEpisode.value.title)
    console.log('Cast API response:', res)
    if (res.code === 0) isCasting.value = true
    else console.error('Cast failed:', res.message)
  } catch (e) {
    console.error('Cast error:', e)
  }
}

async function handleControl(action: string) {
  if (!selectedUdn.value) return
  let realAction = action
  let target: string | undefined
  let volume: number | undefined

  if (action.startsWith('seek:')) {
    realAction = 'seek'
    target = action.slice(5)
  } else if (action.startsWith('volume:')) {
    realAction = 'volume'
    volume = parseInt(action.slice(7))
  }

  try {
    await castApi.control(selectedUdn.value, realAction, target, volume)
    if (realAction === 'stop') isCasting.value = false
  } catch (e) {
    console.error('Control error:', e)
  }
}

onMounted(() => loadDevices())
</script>