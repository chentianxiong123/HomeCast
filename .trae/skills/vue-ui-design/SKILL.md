---
name: "vue-ui-design"
description: "提供Vue3 + NaiveUI的UI设计最佳实践、组件规范和样式指南。当需要设计现代化、高级感的前端界面时调用。"
---

# Vue UI 设计技能

## 技术栈
- **框架**: Vue 3 + TypeScript
- **UI库**: Naive UI
- **构建**: Vite
- **样式**: CSS变量 + Naive主题系统

## 核心设计理念

### 1. 简洁优先
- 避免过度设计，保持界面清爽
- 留白充足，呼吸感强
- 色彩克制，主色调不超过3种

### 2. 一致性原则
- 所有按钮圆角统一 (6-8px)
- 间距遵循8px网格系统
- 字体层级清晰：标题/正文/辅助文字

### 3. 响应式布局
- 移动端优先设计
- 断点：sm(640px) / md(768px) / lg(1024px) / xl(1280px)

## NaiveUI 组件使用规范

### 按钮 Button
```vue
<!-- 主按钮 - 最重要的操作 -->
<n-button type="primary" size="medium">确定</n-button>

<!-- 次要按钮 - 辅助操作 -->
<n-button type="default" size="medium">取消</n-button>

<!-- 文字按钮 - 低频操作 -->
<n-button text>查看更多</n-button>

<!-- 图标按钮 -->
<n-button circle>
  <template #icon>
    <n-icon><SearchOutline /></n-icon>
  </template>
</n-button>
```

### 布局 Layout
```vue
<!-- 弹性布局 - 最常用的布局方式 -->
<n-space vertical size="large">
  <n-card title="卡片标题">
    内容区域
  </n-card>
</n-space>

<!-- 栅格系统 - 复杂页面布局 -->
<n-grid :cols="3" :x-gap="12" :y-gap="12">
  <n-grid-item>
    <n-card>卡片1</n-card>
  </n-grid-item>
  <n-grid-item>
    <n-card>卡片2</n-card>
  </n-grid-item>
</n-grid>
```

### 表单 Form
```vue
<n-form
  :model="formData"
  :rules="rules"
  label-placement="left"
  label-width="100"
  size="medium"
>
  <n-form-item label="用户名" path="username">
    <n-input v-model:value="formData.username" placeholder="请输入" />
  </n-form-item>
  
  <n-form-item>
    <n-space>
      <n-button type="primary" @click="submit">提交</n-button>
      <n-button @click="reset">重置</n-button>
    </n-space>
  </n-form-item>
</n-form>
```

### 数据表格 Table
```vue
<n-data-table
  :columns="columns"
  :data="data"
  :pagination="pagination"
  :bordered="false"
  :single-line="false"
  striped
/>
```

### 反馈组件
```typescript
// 消息提示
const message = useMessage()
message.success('操作成功')
message.error('操作失败')
message.warning('警告信息')

// 对话框
const dialog = useDialog()
dialog.info({
  title: '确认',
  content: '确定要删除吗？',
  positiveText: '确定',
  negativeText: '取消',
  onPositiveClick: () => {
    // 确认操作
  }
})

// 通知
const notification = useNotification()
notification.success({
  title: '成功',
  content: '数据保存成功'
})
```

## 主题定制

### 配置主题
```typescript
import { createApp } from 'vue'
import { NConfigProvider, darkTheme } from 'naive-ui'
import App from './App.vue'

const app = createApp(App)
app.use(NConfigProvider, {
  theme: darkTheme,  // 暗黑主题
  size: 'medium',     // 默认尺寸
})
app.mount('#app')
```

### 自定义主题色
```typescript
import { createTheme } from 'naive-ui'

const customTheme = createTheme({
  common: {
    primaryColor: '#722ed1',
    primaryColorHover: '#9254de',
    primaryColorPressed: '#531dab',
    primaryColorSuppl: '#722ed1',
  }
})
```

## 推荐配色方案

### 深色主题（推荐用于音乐类应用）
```css
:root {
  --primary-color: #ff6b6b;      /* 珊瑚红 - 活力 */
  --bg-color: #0f0f23;           /* 深蓝黑背景 */
  --surface-color: #1a1a2e;      /* 卡片背景 */
  --text-primary: #ffffff;       /* 主文字 */
  --text-secondary: #a0a0b0;     /* 次要文字 */
  --accent-color: #ffd93d;       /* 点缀色 */
}
```

### 浅色主题
```css
:root {
  --primary-color: #1890ff;
  --bg-color: #f5f7fa;
  --surface-color: #ffffff;
  --text-primary: #1f2937;
  --text-secondary: #6b7280;
  --border-color: #e5e7eb;
}
```

## 高级技巧

### 1. 暗黑模式切换
```vue
<template>
  <n-config-provider :theme="currentTheme">
    <n-button @click="toggleTheme">
      {{ isDark ? '☀️' : '🌙' }}
    </n-button>
  </n-config-provider>
</template>

<script setup>
import { ref, computed } from 'vue'
import { darkTheme, lightTheme } from 'naive-ui'

const isDark = ref(false)
const currentTheme = computed(() => isDark.value ? darkTheme : lightTheme)
const toggleTheme = () => isDark.value = !isDark.value
</script>
```

### 2. 全局消息提示
```vue
<!-- App.vue -->
<template>
  <n-config-provider>
    <n-message-provider>
      <n-dialog-provider>
        <n-notification-provider>
          <router-view />
        </n-notification-provider>
      </n-dialog-provider>
    </n-message-provider>
  </n-config-provider>
</template>
```

### 3. 按需加载
```typescript
// 推荐方式 - 自动按需引入
// vite.config.ts
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import Components from 'unplugin-vue-components/vite'
import { NaiveUiResolver } from 'unplugin-vue-components/resolvers'

export default defineConfig({
  plugins: [
    vue(),
    Components({
      resolvers: [NaiveUiResolver()]
    })
  ]
})
```

## 性能优化

### 1. 表格虚拟滚动
```vue
<n-data-table
  :columns="columns"
  :data="largeData"
  virtual-scroll
  :max-height="500"
/>
```

### 2. 图片懒加载
```vue
<n-image
  :src="imageSrc"
  lazy
  preview-src="原始大图地址"
/>
```

### 3. 骨架屏
```vue
<n-skeleton text :repeat="5" />
```

## 常见布局模板

### 侧边栏 + 主内容布局
```vue
<n-layout has-sider style="height: 100vh">
  <n-layout-sider
    bordered
    collapse-mode="width"
    :collapsed-width="64"
    :width="240"
    show-trigger
  >
    <n-menu :options="menuOptions" />
  </n-layout-sider>
  
  <n-layout>
    <n-layout-header bordered style="height: 64px; padding: 0 24px">
      <!-- 顶部导航 -->
    </n-layout-header>
    
    <n-layout-content content-style="padding: 24px;">
      <!-- 主内容区 -->
    </n-layout-content>
  </n-layout>
</n-layout>
```

## 参考资源

- NaiveUI 官方文档: https://www.naiveui.com/
- Vue3 文档: https://cn.vuejs.org/
- 图标库: @vicons/ionicons5 / @vicons/antd / @vicons/material
