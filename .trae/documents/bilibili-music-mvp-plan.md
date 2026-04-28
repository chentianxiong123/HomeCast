# Bilibili Music MVP 项目搭建计划

## 项目概述
基于 **Go 后端 + Vue 前端** 的 Bilibili 音乐播放器跨平台解决方案。

---

## 技术栈

### 后端
- **语言**: Go 1.21+
- **Web 框架**: Gin
- **数据库**: MySQL 8.0
- **缓存**: Redis
- **ORM**: GORM

### 前端
- **框架**: Vue 3.4 + TypeScript
- **构建**: Vite 5
- **状态管理**: Pinia
- **UI 组件**: Naive UI
- **CSS**: TailwindCSS
- **桌面端**: Tauri 2
- **移动端**: Capacitor 6

---

## 项目结构

```
bilibili-music/
├── backend/                 # Go 后端
│   ├── cmd/server/
│   ├── internal/
│   │   ├── api/handler/     # HTTP处理器
│   │   ├── service/         # 业务逻辑
│   │   ├── repository/      # 数据访问
│   │   ├── model/           # 数据模型
│   │   └── pkg/bilibili/    # B站API封装
│   ├── configs/
│   ├── deployments/
│   ├── go.mod
│   └── Makefile
│
└── frontend/                # Vue 前端
    ├── src/
    │   ├── api/             # API封装
    │   ├── core/            # 核心功能
    │   ├── components/      # 组件
    │   ├── views/           # 页面
    │   ├── stores/          # Pinia状态
    │   ├── platform/        # 平台适配
    │   └── utils/           # 工具
    ├── src-tauri/           # Tauri桌面端
    ├── android/             # Capacitor Android
    ├── ios/                 # Capacitor iOS
    └── package.json
```

---

## Phase 1: Go 后端基础 (Day 1-3)

### 任务 1.1: 初始化 Go 项目
- [ ] 创建项目目录结构
- [ ] 初始化 go.mod
- [ ] 安装依赖 (gin, gorm, redis, viper, zap)
- [ ] 创建 Makefile

### 任务 1.2: 配置文件
- [ ] 配置读取 (Viper)
- [ ] config.yaml 配置文件
- [ ] 环境变量支持

### 任务 1.3: 数据库连接
- [ ] MySQL 连接
- [ ] Redis 连接
- [ ] GORM 初始化

### 任务 1.4: HTTP 服务器
- [ ] Gin 服务器启动
- [ ] 中间件配置 (CORS, Logger, Recovery)
- [ ] 健康检查接口

---

## Phase 2: Bilibili API 封装 (Day 3-5)

### 任务 2.1: B站 API 客户端
- [ ] 创建 bilibili 包
- [ ] HTTP 客户端封装
- [ ] 请求头配置 (User-Agent, Referer)

### 任务 2.2: 搜索接口
- [ ] 搜索音乐 API
- [ ] 参数处理
- [ ] 响应解析

### 任务 2.3: 视频信息接口
- [ ] 获取视频详情
- [ ] 提取 CID
- [ ] 获取封面/标题/UP主

### 任务 2.4: 音频流接口
- [ ] 获取播放地址
- [ ] 音质选择 (128K/192K/320K/FLAC)
- [ ] 流式代理

---

## Phase 3: 后端业务实现 (Day 5-7)

### 任务 3.1: 音乐服务
- [ ] MusicService 接口定义
- [ ] 搜索服务实现
- [ ] 音频流服务实现
- [ ] 缓存集成

### 任务 3.2: HTTP Handler
- [ ] SearchHandler
- [ ] StreamHandler
- [ ] 响应封装

### 任务 3.3: 路由配置
- [ ] API 路由
- [ ] 版本控制 (v1)
- [ ] 文档注释

### 任务 3.4: 歌词服务 (可选)
- [ ] 歌词获取
- [ ] 歌词解析
- [ ] 缓存存储

---

## Phase 4: Vue 前端初始化 (Day 7-9)

### 任务 4.1: 项目初始化
- [ ] Vite + Vue3 + TS 项目
- [ ] 安装依赖
- [ ] 目录结构创建

### 任务 4.2: 配置
- [ ] Vite 配置
- [ ] TypeScript 配置
- [ ] TailwindCSS 配置
- [ ] ESLint + Prettier

### 任务 4.3: 基础架构
- [ ] 路由配置 (Vue Router)
- [ ] 状态管理 (Pinia)
- [ ] API 请求封装 (Axios)
- [ ] 类型定义

### 任务 4.4: UI 框架
- [ ] Naive UI 安装配置
- [ ] 主题配置
- [ ] 基础布局组件

---

## Phase 5: 核心播放器 (Day 9-12)

### 任务 5.1: 播放器核心
- [ ] Audio 元素封装
- [ ] 播放/暂停/停止
- [ ] 进度控制
- [ ] 音量控制

### 任务 5.2: 播放器状态
- [ ] PlayerStore (Pinia)
- [ ] 响应式状态
- [ ] 持久化

### 任务 5.3: 播放列表
- [ ] PlaylistStore
- [ ] 添加/删除/排序
- [ ] 播放模式

### 任务 5.4: 播放器 UI
- [ ] PlayerBar 组件
- [ ] 进度条组件
- [ ] 控制按钮
- [ ] 音量控制

---

## Phase 6: 搜索功能 (Day 12-14)

### 任务 6.1: 搜索 API
- [ ] 搜索接口封装
- [ ] 搜索历史
- [ ] 热门搜索

### 任务 6.2: 搜索页面
- [ ] SearchView
- [ ] 搜索框组件
- [ ] 搜索结果列表

### 任务 6.3: 歌曲项组件
- [ ] SongItem 组件
- [ ] 封面显示
- [ ] 操作按钮 (播放/添加到列表)

### 任务 6.4: 播放集成
- [ ] 点击播放
- [ ] 添加到播放列表
- [ ] 双击播放

---

## Phase 7: 播放列表页面 (Day 14-16)

### 任务 7.1: 播放列表视图
- [ ] PlaylistView
- [ ] 当前播放列表
- [ ] 历史记录

### 任务 7.2: 列表管理
- [ ] 拖拽排序
- [ ] 删除歌曲
- [ ] 清空列表

### 任务 7.3: 播放模式
- [ ] 顺序播放
- [ ] 列表循环
- [ ] 单曲循环
- [ ] 随机播放

---

## Phase 8: Tauri 桌面端 (Day 16-18)

### 任务 8.1: Tauri 初始化
- [ ] 安装 Tauri CLI
- [ ] 初始化项目
- [ ] 配置 tauri.conf.json

### 任务 8.2: 桌面适配
- [ ] 窗口配置
- [ ] 系统托盘
- [ ] 全局快捷键

### 任务 8.3: 后台播放
- [ ] 后台播放支持
- [ ] 系统媒体控制
- [ ] 通知集成

### 任务 8.4: 打包
- [ ] Windows 打包
- [ ] macOS 打包
- [ ] Linux 打包

---

## Phase 9: Capacitor 移动端 (Day 18-21)

### 任务 9.1: Capacitor 初始化
- [ ] 安装 Capacitor
- [ ] 初始化项目
- [ ] 配置 capacitor.config.ts

### 任务 9.2: 平台添加
- [ ] 添加 Android 平台
- [ ] 添加 iOS 平台
- [ ] 同步 Web 代码

### 任务 9.3: 移动端适配
- [ ] 响应式布局优化
- [ ] 触摸事件
- [ ] 移动端UI调整

### 任务 9.4: 原生功能
- [ ] 后台播放插件
- [ ] 媒体通知
- [ ] 本地存储

---

## Phase 10: 优化与部署 (Day 21-25)

### 任务 10.1: 性能优化
- [ ] 代码分割
- [ ] 懒加载
- [ ] 缓存策略

### 任务 10.2: 错误处理
- [ ] 全局错误捕获
- [ ] 加载状态
- [ ] 重试机制

### 任务 10.3: 部署准备
- [ ] Docker 配置
- [ ] docker-compose.yml
- [ ] Nginx 配置

### 任务 10.4: 文档
- [ ] API 文档
- [ ] 部署文档
- [ ] 使用说明

---

## 里程碑检查点

| 阶段 | 检查内容 | 验收标准 |
|------|----------|----------|
| Phase 3 | 后端 API | 搜索/播放API可用 |
| Phase 6 | Web 端 | 搜索/播放功能可用 |
| Phase 8 | 桌面端 | Win/Mac 可运行 |
| Phase 9 | 移动端 | Android 可运行 |
| Phase 10 | 完整 MVP | 所有平台可用 |

---

## 风险与应对

| 风险 | 影响 | 应对措施 |
|------|------|----------|
| B站 API 变动 | 高 | 封装接口层，便于修改 |
| 跨域问题 | 中 | Go 后端代理 |
| 移动端性能 | 中 | 使用虚拟列表，懒加载 |
| 音频格式兼容 | 中 | 多格式备选 |

---

## 下一步行动

1. 等待计划确认
2. 开始 Phase 1: Go 后端初始化
3. 并行准备前端环境
