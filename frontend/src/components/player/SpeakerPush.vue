<template>
  <div>
    <!-- 未登录状态：显示登录按钮 -->
    <n-button
      v-if="!speakerState.isLoggedIn"
      type="primary"
      size="small"
      :loading="speakerState.isLoading"
      @click="showLoginModal = true"
    >
      <template #icon>
        <n-icon><SpeakerIcon /></n-icon>
      </template>
      登录小爱
    </n-button>

    <!-- 已登录状态：显示推送下拉菜单 -->
    <n-dropdown
      v-else
      :options="dropdownOptions"
      @select="handleSelect"
      placement="top-end"
    >
      <n-button :type="speakerState.isPushing ? 'error' : 'primary'" size="small">
        <template #icon>
          <n-icon><SpeakerIcon /></n-icon>
        </template>
        {{ speakerState.isPushing ? '停止推送' : '推送' }}
      </n-button>
    </n-dropdown>

    <!-- 登录对话框 -->
    <n-modal
      v-model:show="showLoginModal"
      title="登录小米账号"
      preset="card"
      style="width: 450px"
      :mask-closable="false"
      @update:show="onModalClose"
    >
      <n-tabs v-model:value="loginTab" type="line" @update:value="onTabChange">
        <!-- 二维码登录 -->
        <n-tab-pane name="qrcode" tab="扫码登录">
          <div style="text-align: center; padding: 20px;">
            <!-- 二维码显示 -->
            <div v-if="qrState.qrImage" style="margin-bottom: 16px;">
              <img
                :src="qrState.qrImage"
                alt="二维码"
                style="width: 200px; height: 200px; border: 1px solid #e0e0e0;"
              />
            </div>
            <div v-else style="width: 200px; height: 200px; margin: 0 auto; background: #f5f5f5; display: flex; align-items: center; justify-content: center; border: 1px solid #e0e0e0;">
              <n-spin v-if="speakerState.isLoading" size="large" />
              <span v-else style="color: #999;">点击生成二维码</span>
            </div>

            <!-- 状态提示 -->
            <n-alert
              :type="qrState.status === 'success' ? 'success' : qrState.status === 'failed' ? 'error' : 'info'"
              :show-icon="false"
              style="margin-top: 16px;"
            >
              {{ qrState.message || '请使用米家 APP 扫描二维码' }}
            </n-alert>

            <!-- 操作按钮 -->
            <n-space justify="center" style="margin-top: 16px;">
              <n-button
                type="primary"
                :loading="speakerState.isLoading"
                @click="startQRLogin"
                :disabled="qrState.status === 'pending'"
              >
                {{ qrState.qrImage ? '重新生成' : '生成二维码' }}
              </n-button>
            </n-space>
          </div>
        </n-tab-pane>

        <!-- 账号密码登录 -->
        <n-tab-pane name="account" tab="账号密码">
          <n-form
            ref="accountFormRef"
            :model="accountForm"
            :rules="accountRules"
            label-placement="left"
            label-width="80px"
          >
            <n-form-item label="小米账号" path="account">
              <n-input
                v-model:value="accountForm.account"
                placeholder="手机号/邮箱"
                @keyup.enter="handleLogin"
              />
            </n-form-item>
            <n-form-item label="密码" path="password">
              <n-input
                v-model:value="accountForm.password"
                type="password"
                placeholder="请输入密码"
                show-password-on="mousedown"
                @keyup.enter="handleLogin"
              />
            </n-form-item>
          </n-form>
        </n-tab-pane>

        <!-- Cookie 登录 -->
        <n-tab-pane name="cookie" tab="Cookie登录">
          <n-alert type="info" :show-icon="false" style="margin-bottom: 16px;">
            <template #default>
              <div style="font-size: 12px;">
                当账号密码触发验证码时，请使用 Cookie 登录<br>
                1. 浏览器访问 <a href="https://account.xiaomi.com" target="_blank">account.xiaomi.com</a> 并登录<br>
                2. F12 → Application → Cookies → 复制 userId 和 passToken<br>
                3. 格式: userId=xxx; passToken=yyy
              </div>
            </template>
          </n-alert>
          <n-form
            ref="cookieFormRef"
            :model="cookieForm"
            :rules="cookieRules"
            label-placement="top"
          >
            <n-form-item label="Cookie" path="cookie">
              <n-input
                v-model:value="cookieForm.cookie"
                type="textarea"
                placeholder="userId=xxx; passToken=yyy"
                :rows="3"
              />
            </n-form-item>
          </n-form>
        </n-tab-pane>
      </n-tabs>

      <template #footer>
        <n-space justify="end">
          <n-button @click="showLoginModal = false">取消</n-button>
          <n-button
            v-if="loginTab !== 'qrcode'"
            type="primary"
            :loading="speakerState.isLoading"
            @click="handleLogin"
          >
            登录
          </n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, h, onMounted, onUnmounted, ref } from 'vue'
import { NIcon, useMessage } from 'naive-ui'
import type { FormInst, FormRules } from 'naive-ui'
import {
  VolumeHighOutline as SpeakerIcon,
  StopCircleOutline as StopIcon,
  RefreshOutline as RefreshIcon,
  LogOutOutline as LogoutIcon,
} from '@vicons/ionicons5'
import type { DropdownOption } from 'naive-ui'
import {
  speakerState,
  loadSpeakerDevices,
  loadSpeakerStatus,
  pushToSpeaker,
  stopPush,
  loginSpeaker,
  logoutSpeaker,
  generateQRCode,
  checkQRStatus,
  resetQRLogin,
} from '../../stores/player'

const message = useMessage()
const showLoginModal = ref(false)
const loginTab = ref('qrcode')  // 默认二维码登录（推荐）
const accountFormRef = ref<FormInst | null>(null)
const cookieFormRef = ref<FormInst | null>(null)

// 二维码状态
const qrState = ref({
  qrImage: '',
  qrUrl: '',
  status: 'idle',  // idle, pending, scanning, success, failed
  message: '',
  userId: '',
})

let qrCheckTimer: number | null = null

const accountForm = ref({
  account: '',
  password: '',
})

const cookieForm = ref({
  cookie: '',
})

const accountRules: FormRules = {
  account: [
    { required: true, message: '请输入小米账号', trigger: 'blur' },
  ],
  password: [
    { required: true, message: '请输入密码', trigger: 'blur' },
  ],
}

const cookieRules: FormRules = {
  cookie: [
    { required: true, message: '请输入 Cookie', trigger: 'blur' },
    {
      validator: (rule: any, value: string) => {
        if (!value || (!value.includes('userId') && !value.includes('passToken'))) {
          return new Error('Cookie 格式不正确，需包含 userId 和 passToken')
        }
        return true
      },
      trigger: 'blur'
    }
  ],
}

const dropdownOptions = computed<DropdownOption[]>(() => {
  const options: DropdownOption[] = []

  // 设备列表
  if (speakerState.speakerDevices.length === 0) {
    options.push({
      label: '暂无设备，请先刷新',
      key: 'empty',
      disabled: true,
    })
  } else {
    // 设备分组（按在线状态）
    const onlineDevices = speakerState.speakerDevices.filter(d => d.is_online !== false)
    const offlineDevices = speakerState.speakerDevices.filter(d => d.is_online === false)

    if (onlineDevices.length > 0) {
      options.push({
        type: 'group',
        label: '在线设备',
        key: 'online-group',
        children: onlineDevices.map((device) => ({
          label: () => h('span', {}, [
            h(NIcon, { size: 14, style: 'margin-right:6px;vertical-align:middle' }, { default: () => h(SpeakerIcon) }),
            device.name,
            h('span', { style: 'color:#999;margin-left:8px;font-size:12px' }, device.hardware),
          ]),
          key: device.did,
        })),
      })
    }

    if (offlineDevices.length > 0) {
      options.push({
        type: 'group',
        label: '离线设备',
        key: 'offline-group',
        children: offlineDevices.map((device) => ({
          label: () => h('span', { style: 'color:#999' }, [
            h(NIcon, { size: 14, style: 'margin-right:6px;vertical-align:middle' }, { default: () => h(SpeakerIcon) }),
            device.name,
            ' (离线)',
          ]),
          key: device.did,
          disabled: true,
        })),
      })
    }
  }

  options.push({ type: 'divider', key: 'd1' })

  // 控制选项
  if (speakerState.isPushing) {
    options.push({
      label: () => h('span', { style: 'color:#e88080' }, [
        h(NIcon, { size: 14, style: 'margin-right:6px;vertical-align:middle' }, { default: () => h(StopIcon) }),
        '停止推送',
      ]),
      key: 'stop',
    })
  }

  options.push({
    label: () => h('span', {}, [
      h(NIcon, { size: 14, style: 'margin-right:6px;vertical-align:middle' }, { default: () => h(RefreshIcon) }),
      '刷新设备列表',
    ]),
    key: 'refresh',
  })

  options.push({ type: 'divider', key: 'd2' })

  options.push({
    label: () => h('span', { style: 'color:#e88080' }, [
      h(NIcon, { size: 14, style: 'margin-right:6px;vertical-align:middle' }, { default: () => h(LogoutIcon) }),
      '退出登录',
    ]),
    key: 'logout',
  })

  return options
})

async function handleSelect(key: string) {
  if (key === 'refresh') {
    await loadSpeakerDevices()
    message.success('设备列表已刷新')
    return
  }

  if (key === 'stop') {
    await stopPush()
    return
  }

  if (key === 'logout') {
    await logoutSpeaker()
    message.success('已退出登录')
    return
  }

  if (key === 'empty') return

  const result = await pushToSpeaker(key)
  if (result) {
    message.success('已开始推送到小爱音箱')
  } else {
    message.error('推送失败')
  }
}

// ========== 二维码登录 ==========
async function startQRLogin() {
  // 清除之前的轮询
  stopQRCheck()

  // 重置状态
  qrState.value = {
    qrImage: '',
    qrUrl: '',
    status: 'idle',
    message: '正在生成二维码...',
    userId: '',
  }

  // 调用后端生成二维码
  const res = await generateQRCode()

  if (res.code !== 0) {
    qrState.value.status = 'failed'
    qrState.value.message = res.message || '生成二维码失败'
    return
  }

  // 如果已经登录
  if (res.data?.is_logged_in) {
    qrState.value.status = 'success'
    qrState.value.message = '已登录'
    showLoginModal.value = false
    message.success('登录成功')
    return
  }

  // 显示二维码
  qrState.value.qrImage = res.data?.qr_image || ''
  qrState.value.qrUrl = res.data?.qr_url || ''
  qrState.value.status = 'pending'
  qrState.value.message = '请使用米家 APP 扫描二维码'

  // 开始轮询检查状态
  startQRCheck()
}

function startQRCheck() {
  // 每 2 秒检查一次状态
  qrCheckTimer = window.setInterval(async () => {
    const res = await checkQRStatus()

    if (res.code !== 0) {
      qrState.value.status = 'failed'
      qrState.value.message = res.message || '检查状态失败'
      stopQRCheck()
      return
    }

    const status = res.data?.status
    qrState.value.status = status
    qrState.value.message = res.data?.message || ''
    qrState.value.userId = res.data?.user_id || ''

    if (status === 'success') {
      // 登录成功
      stopQRCheck()
      showLoginModal.value = false
      message.success('登录成功')
      // 重置状态
      qrState.value = {
        qrImage: '',
        qrUrl: '',
        status: 'idle',
        message: '',
        userId: '',
      }
    } else if (status === 'failed') {
      // 登录失败
      stopQRCheck()
    }
  }, 2000)
}

function stopQRCheck() {
  if (qrCheckTimer) {
    clearInterval(qrCheckTimer)
    qrCheckTimer = null
  }
}

function onTabChange(tab: string) {
  // 切换到非二维码标签时，停止轮询
  if (tab !== 'qrcode') {
    stopQRCheck()
  }
}

function onModalClose(show: boolean) {
  if (!show) {
    // 关闭模态框时停止轮询
    stopQRCheck()
    // 如果正在等待扫码，重置后端状态
    if (qrState.value.status === 'pending') {
      resetQRLogin()
    }
  }
}

// ========== 账号密码/Cookie 登录 ==========
async function handleLogin() {
  if (loginTab.value === 'account') {
    if (!accountFormRef.value) return
    await accountFormRef.value.validate(async (errors) => {
      if (errors) return

      const result = await loginSpeaker(accountForm.value.account, accountForm.value.password)
      if (result.success) {
        message.success(result.message)
        showLoginModal.value = false
        accountForm.value.account = ''
        accountForm.value.password = ''
      } else {
        message.error(result.message)
      }
    })
  } else {
    if (!cookieFormRef.value) return
    await cookieFormRef.value.validate(async (errors) => {
      if (errors) return

      const result = await loginSpeaker('', '', cookieForm.value.cookie)
      if (result.success) {
        message.success(result.message)
        showLoginModal.value = false
        cookieForm.value.cookie = ''
      } else {
        message.error(result.message)
      }
    })
  }
}

onMounted(() => {
  // 页面加载时检查登录状态
  loadSpeakerStatus()
})

onUnmounted(() => {
  // 组件卸载时停止轮询
  stopQRCheck()
})
</script>
