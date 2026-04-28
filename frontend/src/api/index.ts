export interface DLNADevice {
  name: string
  udn: string
  ip: string
  port: number
  device_type: string
}

export interface SpeakerDevice {
  did: string
  name: string
  hardware: string
  device_id?: string
  is_online?: boolean
}

export interface Episode {
  index: number
  title: string
  url: string
  duration: number
  thumbnail?: string
}

export interface SniffResult {
  title: string
  sniff_method: string
  episodes: Episode[]
  episodes_list?: Episode[]  // 集数列表（详情页返回）
}

export interface Site {
  name: string
  url: string
  site_type: string
}

export interface SiteData {
  sites: Site[]
  preset: Site[]
}

const USE_MOCK = false

function delay(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms))
}

export const cacheApi = {
  async getList() {
    if (USE_MOCK) {
      await delay(300)
      return { code: 0, data: { items: [], summary: { count: 0, total_mb: 0, max_mb: 500 } } }
    }
    try {
      const axios = (await import('axios')).default
      const res = await axios.get('/api/v1/cache/list')
      return res.data
    } catch (e: any) {
      return { code: -1, message: e.message || '请求失败' }
    }
  },

  async delete(bvid: string) {
    try {
      const axios = (await import('axios')).default
      const res = await axios.delete(`/api/v1/cache/${bvid}`)
      return res.data
    } catch (e: any) {
      return { code: -1, message: e.message || '删除失败' }
    }
  },

  async clearAll() {
    try {
      const axios = (await import('axios')).default
      const res = await axios.delete('/api/v1/cache/clear/all')
      return res.data
    } catch (e: any) {
      return { code: -1, message: e.message || '清空失败' }
    }
  },

  async create(bvid: string) {
    try {
      const axios = (await import('axios')).default
      const res = await axios.post(`/api/v1/cache/create/${bvid}`)
      return res.data
    } catch (e: any) {
      return { code: -1, message: e.message || '创建失败' }
    }
  },
}

export const speakerApi = {
  async getStatus() {
    if (USE_MOCK) {
      await delay(200)
      return { code: 0, data: { is_logged_in: false, device_count: 0, devices: [] }, message: 'success' }
    }
    try {
      const axios = (await import('axios')).default
      const res = await axios.get('/api/v1/speaker/status')
      return res.data
    } catch (e: any) {
      return { code: -1, message: e.message || '请求失败' }
    }
  },

  // 账号密码/Cookie 登录
  async login(account: string, password: string, cookie: string = '') {
    if (USE_MOCK) {
      await delay(1000)
      return { code: 0, message: '登录成功', data: { device_count: 2, account: account || 'Cookie登录' } }
    }
    try {
      const axios = (await import('axios')).default
      const res = await axios.post('/api/v1/speaker/login', { account, password, cookie })
      return res.data
    } catch (e: any) {
      return { code: -1, message: e.message || '登录失败' }
    }
  },

  // 二维码登录
  async generateQRCode() {
    if (USE_MOCK) {
      await delay(500)
      return { code: 0, message: '请扫码', data: { is_logged_in: false, qr_image: '', qr_url: '' } }
    }
    try {
      const axios = (await import('axios')).default
      const res = await axios.post('/api/v1/speaker/qr/generate')
      return res.data
    } catch (e: any) {
      return { code: -1, message: e.message || '生成二维码失败' }
    }
  },

  async checkQRStatus() {
    if (USE_MOCK) {
      await delay(500)
      return { code: 0, message: '等待扫码', data: { status: 'pending', user_id: '' } }
    }
    try {
      const axios = (await import('axios')).default
      const res = await axios.get('/api/v1/speaker/qr/status')
      return res.data
    } catch (e: any) {
      return { code: -1, message: e.message || '检查状态失败' }
    }
  },

  async resetQRLogin() {
    try {
      const axios = (await import('axios')).default
      const res = await axios.post('/api/v1/speaker/qr/reset')
      return res.data
    } catch (e: any) {
      return { code: -1, message: e.message || '重置失败' }
    }
  },

  async logout() {
    if (USE_MOCK) {
      await delay(200)
      return { code: 0, message: '已退出登录' }
    }
    try {
      const axios = (await import('axios')).default
      const res = await axios.post('/api/v1/speaker/logout')
      return res.data
    } catch (e: any) {
      return { code: -1, message: e.message || '退出失败' }
    }
  },

  async getDevices() {
    if (USE_MOCK) {
      await delay(300)
      return { code: 0, data: (await import('../mock/data')).MOCK_SPEAKER_DEVICES, message: 'success' }
    }
    try {
      const axios = (await import('axios')).default
      const res = await axios.get('/api/v1/speaker/devices')
      return res.data
    } catch (e: any) {
      return { code: -1, message: e.message || '请求失败' }
    }
  },

  async play(deviceDid: string, bvid: string) {
    if (USE_MOCK) {
      await delay(500)
      return { code: 0, data: { proxy_url: `/proxy/audio/mock_${bvid}` }, message: 'success' }
    }
    try {
      const axios = (await import('axios')).default
      const res = await axios.post('/api/v1/speaker/play', { bvid, did: deviceDid, quality: 30280 })
      return res.data
    } catch (e: any) {
      return { code: -1, message: e.message || '推送失败' }
    }
  },

  async control(deviceDid: string, action: string, volume?: number) {
    if (USE_MOCK) {
      await delay(200)
      return { code: 0, message: 'success' }
    }
    try {
      const axios = (await import('axios')).default
      const res = await axios.post('/api/v1/speaker/control', { did: deviceDid, action, volume })
      return res.data
    } catch (e: any) {
      return { code: -1, message: e.message || '控制失败' }
    }
  },

  async getPlayerStatus(did: string) {
    if (USE_MOCK) {
      await delay(100)
      return { code: 0, data: { status: 1, volume: 50 }, message: 'success' }
    }
    try {
      const axios = (await import('axios')).default
      const res = await axios.get(`/api/v1/speaker/player_status/${did}`)
      return res.data
    } catch (e: any) {
      return { code: -1, message: e.message || '获取状态失败' }
    }
  },

  async getVolume(did: string) {
    if (USE_MOCK) {
      await delay(100)
      return { code: 0, data: { volume: 50 }, message: 'success' }
    }
    try {
      const axios = (await import('axios')).default
      const res = await axios.get(`/api/v1/speaker/volume/${did}`)
      return res.data
    } catch (e: any) {
      return { code: -1, message: e.message || '获取音量失败' }
    }
  },

  async setVolume(did: string, volume: number) {
    if (USE_MOCK) {
      await delay(100)
      return { code: 0, data: { volume }, message: 'success' }
    }
    try {
      const axios = (await import('axios')).default
      const res = await axios.post('/api/v1/speaker/volume', { did, volume })
      return res.data
    } catch (e: any) {
      return { code: -1, message: e.message || '设置音量失败' }
    }
  },
}

export const castApi = {
  async sniff(url: string) {
    if (!url) return { code: -1, message: '请输入URL' }
    if (USE_MOCK) {
      await delay(800)
      return { code: 0, data: (await import('../mock/data')).MOCK_SNIFF_RESULT, message: 'success' }
    }
    try {
      const axios = (await import('axios')).default
      const res = await axios.post('/api/v1/cast/sniff', { url }, { timeout: 120000 })
      return res.data
    } catch (e: any) {
      if (e.code === 'ECONNABORTED') {
        return { code: -1, message: '请求超时，请重试' }
      }
      return { code: -1, message: e.message || '嗅探失败' }
    }
  },

  async playUrl(url: string, title = 'Video') {
    try {
      const axios = (await import('axios')).default
      const res = await axios.post('/api/v1/cast/play_url', { url, title })
      return res.data
    } catch (e: any) {
      return { code: -1, message: e.message || '获取播放链接失败' }
    }
  },

  async getDevices() {
    if (USE_MOCK) {
      await delay(400)
      return { code: 0, data: (await import('../mock/data')).MOCK_DLNA_DEVICES, message: 'success' }
    }
    try {
      const axios = (await import('axios')).default
      const res = await axios.get('/api/v1/cast/devices', { timeout: 15000 })
      return res.data
    } catch (e: any) {
      return { code: -1, data: [], message: e.message || '获取设备失败' }
    }
  },

  async start(episodeUrl: string, deviceUdn: string, title = 'Video') {
    if (USE_MOCK) {
      await delay(600)
      return { code: 0, data: { proxy_url: '/proxy/video/mock_cast', device_name: 'Mock TV' }, message: 'success' }
    }
    try {
      const axios = (await import('axios')).default
      const res = await axios.post('/api/v1/cast/start', { episode_url: episodeUrl, device_udn: deviceUdn, title })
      return res.data
    } catch (e: any) {
      return { code: -1, message: e.message || '投屏失败' }
    }
  },

  async control(deviceUdn: string, action: string, target?: string, volume?: number) {
    if (USE_MOCK) {
      await delay(200)
      return { code: 0, message: 'success' }
    }
    try {
      const axios = (await import('axios')).default
      const res = await axios.post('/api/v1/cast/control', { device_udn: deviceUdn, action, target, volume })
      return res.data
    } catch (e: any) {
      return { code: -1, message: e.message || '控制失败' }
    }
  },

  async getStatus(deviceUdn: string) {
    if (USE_MOCK) {
      await delay(100)
      return {
        code: 0,
        data: {
          transport: { state: 'PLAYING' },
          position: { rel_time: '00:05:23', duration: '00:45:20' },
        },
        message: 'success',
      }
    }
    try {
      const axios = (await import('axios')).default
      const res = await axios.get(`/api/v1/cast/status/${deviceUdn}`)
      return res.data
    } catch (e: any) {
      return { code: -1, data: null, message: e.message || '获取状态失败' }
    }
  },
}

export const favlistApi = {
  async getList() {
    if (USE_MOCK) {
      await delay(200)
      return { code: 0, data: (await import('../mock/data')).MOCK_FAVLISTS, message: 'success' }
    }
    try {
      const axios = (await import('axios')).default
      const res = await axios.get('/api/v1/favlist/list')
      return res.data
    } catch (e: any) {
      return { code: -1, data: [], message: e.message || '获取失败' }
    }
  },

  async getInfo(mid: number) {
    if (USE_MOCK) {
      await delay(300)
      return { code: 0, data: (await import('../mock/data')).MOCK_FAV_INFO, message: 'success' }
    }
    try {
      const axios = (await import('axios')).default
      const res = await axios.get(`/api/v1/favlist/info/${mid}`)
      return res.data
    } catch (e: any) {
      return { code: -1, data: null, message: e.message || '获取失败' }
    }
  },
}

export const sitesApi = {
  async getList() {
    try {
      const axios = (await import('axios')).default
      const res = await axios.get('/api/v1/sites/list')
      return res.data
    } catch (e: any) {
      return { code: -1, message: e.message || '获取网站列表失败' }
    }
  },

  async add(site: { name: string; url: string; site_type?: string }) {
    try {
      const axios = (await import('axios')).default
      const res = await axios.post('/api/v1/sites/add', site)
      return res.data
    } catch (e: any) {
      return { code: -1, message: e.message || '添加网站失败' }
    }
  },

  async remove(url: string) {
    try {
      const axios = (await import('axios')).default
      const res = await axios.post('/api/v1/sites/remove', null, { params: { url } })
      return res.data
    } catch (e: any) {
      return { code: -1, message: e.message || '删除网站失败' }
    }
  },
}
