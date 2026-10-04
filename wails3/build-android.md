# 🤖 安卓端构建（统一壳 wails3/）

产物：`wails3/bin/hcexp.apk`

## 命令

```bash
cd wails3

# x86_64 模拟器（waydroid）
task android:package ARCH=amd64

# arm64 真机（给爸妈手机装）
task android:package ARCH=arm64
```

装进设备：`adb install -r bin/hcexp.apk`

## 已知约束

- **包名锁死 `com.wails.app`**：wails3 安卓 JNI export 符号写死（`Java_com_wails_app_*`），改包名要 fork 依赖 → 保持默认；显示名由 manifest label 控制 = HomeCast
- **asset 桥三丢**：WebViewAssetLoader.PathHandler 剥 query、header 不透传、body 丢弃 → 前端参数一律走 query + `hc=1` 标记（搜索/收藏/歌词源已按此绕）
- **APK 被冻结**（报 task 挂）：`adb reboot` 清
- **数据落盘**：`filesDir/.config/homecast/`（hc-http.log + homecast.db）

## 挂件（安卓悬浮窗，待写）

`widget_android.go` 目前 no-op。悬浮窗方案（已定）：
- Java 层 `TYPE_APPLICATION_OVERLAY` 悬浮窗 Service + `SYSTEM_ALERT_WINDOW` 授权引导
- 数据源 = 轮询本地 Go 后端（复用挂件 API）
- 显示层实现在 `widget/display_android.go`（待写）