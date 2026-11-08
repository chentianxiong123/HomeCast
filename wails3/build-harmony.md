# 🧭 鸿蒙版：从编译、签名到模拟器跑通（实测复现手册，2026-11-08）

产物：**`com.homecast.music` HAP**（bundleName `com.homecast.music`，EntryAbility）。
本文档是**实测复现路径**：以下每一步都在 win11 DevEco + OpenHarmony x86_64 模拟器上验证过（2026-11-08，Go 后端在 HAP 内启动、HTTP 就绪、ArkWeb 页面显示成功）。

> 本文档前身是「规划」版（2026-11-08 沉淀）。落地后整体升级为可照做的复现手册；
> 规划期内容（ohos-go 选型、wails3 不支持鸿蒙的结论）保留在「核心理念 / 环境」节。

## 核心理念（沿用项目原则，落地未变）

- **前端↔后端真实 HTTP**（`http://127.0.0.1:28976`），壳只当系统壳（WebView / Native 加载 / 系统能力）
- **逻辑一份住后端（Go）**——鸿蒙版后端 = 现有 `go/cmd/server`，**不改业务逻辑**，前端复用 go:embed 的 HTML/JS/CSS
- **不用 wails3 壳**（wails3 官方不支持鸿蒙），壳用 ArkTS 自建：ArkWeb + NAPI 加载 Go c-shared 库

## 环境（win11 鸿蒙开发机）

| 项 | 值 |
|---|---|
| DevEco Studio | 26.0（`C:\PROGRA~1\Huawei\DevEco~1`） |
| OpenHarmony SDK | `sdk\default\openharmony`（26.0.0，native/llvm、toolchains/lib 内含 hap-sign-tool） |
| 模拟器 | Emulator26.0，**x86_64**，OpenHarmony 7.0.0.107，hdc 目标 `127.0.0.1:5555` |
| Go 工具链 | **`hmgo`（star4277/ohos-go，go1.26.5）**——与 win11 bootstrap 的 go1.26.5 匹配 |
| Java | 21（签名工具 hap-sign-tool.jar 运行用） |

Windows 副本仓库：`C:\Users\a1\homecast`（与 Linux 仓库 `/mnt/shared/my-project/mymusic` 同步）。

## 关键文件

```
go/cmd/server/main.go             # !openharmony 普通入口（桌面/安卓/iOS 不变）
go/cmd/server/main_harmony.go     # openharmony：//export StartServer(dataDir)
wails3/build/harmony/              # 鸿蒙壳工程（ArkTS + NAPI + ArkWeb）
  AppScope/app.json5               # bundleName com.homecast.music
  entry/build-profile.json5        # abiFilters: arm64-v8a, x86_64
  entry/src/main/module.json5      # ohos.permission.INTERNET
  entry/src/main/resources/base/profile/network_config.json   # cleartext HTTP 允许
  entry/src/main/ets/entryability/EntryAbility.ets # onCreate 调 startServer(filesDir)
  entry/src/main/ets/pages/Index.ets               # 轮询 HTTP 200 后加载 ArkWeb
  entry/src/main/cpp/napi_init.cpp # NAPI 注册 startServer → 调 Go StartServer
  entry/src/main/cpp/CMakeLists.txt # 根因修复：SKIP_RPATH + IMPORTED_NO_SONAME
```

## 复现步骤

### 1. 编译 Go 后端为鸿蒙 c-shared 库（win11，两条 ABI）

`/tmp/build-harmony-x86_64.bat`：

```bat
@echo off
set "GOROOT=C:\Users\a1\hmgo"
set "GOOS=openharmony"
set "GOARCH=amd64"
set "CGO_ENABLED=1"
set "CC=C:\PROGRA~1\Huawei\DevEco~1\sdk\default\openharmony\native\llvm\bin\clang.exe --target=x86_64-linux-ohos"
set "GOPROXY=https://goproxy.cn,direct"
cd /d C:\Users\a1\homecast\go
C:\Users\a1\hmgo\bin\go.exe build -buildmode=c-shared -o C:\Users\a1\libhomecast_go.so ./cmd/server
```

`/tmp/build-harmony-arm64.bat` 同构，`GOARCH=arm64` + `--target=aarch64-linux-ohos`，输出 `homecast-arm64.so`。

产物（.so + 头文件 `libhomecast_go.h`）复制进壳工程：

```
entry/src/main/cpp/libs/x86_64/libhomecast_go.so
entry/src/main/cpp/libs/arm64-v8a/libhomecast_go.so   # 真机用
entry/src/main/cpp/include/libhomecast_go.h
```

### 2. 构建 HAP（hvigorw，无 daemon）

```bat
C:\PROGRA~1\Huawei\DevEco~1\tools\hvigor\bin\hvigorw.bat --mode module -p product=default assembleHap --no-daemon
```

> 改了 CMakeLists / .cpp 后若没生效，先 `hvigorw clean` 再 assembleHap。
> SDK 26 用独立兼容目录 junction：`C:\Users\a1\homecast-ohos-sdk\26.0.0`（新 hvigor 布局需要，不删原 SDK）。

未签名包：`entry\build\default\outputs\default\entry-default-unsigned.hap`

### 3. 签名（hap-sign-tool，本地 OpenHarmony 调试证书）

签名脚本 `/tmp/sign-homecast-hap.bat`（本地路径 `C:\Users\a1\sign-homecast-hap.bat`）三步：
`sign-profile`（生成 .p7b）→ `sign-app`（对 unsigned.hap，`-compatibleVersion 26 -inForm zip`）→ `verify-app` 校验。
产出：`wails3\build\harmony\signing\homecast-signed.hap`。

**签名材料不提交 git**（`signing/` 已在 `.gitignore`）。

### 4. 安装启动（模拟器）

```bat
hdc list targets                        # 127.0.0.1:5555
hdc install C:\Users\a1\homecast\wails3\build\harmony\signing\homecast-signed.hap
hdc shell aa start -a EntryAbility -b com.homecast.music
hdc shell "hilog | grep HomeCast"       # 看启动/后端日志
```

### 5. 验证（成功日志序列）

hilog 里应依次出现（PID、时间戳随意）：

```
RegisterEntryModule called
NAPI Init called
NAPI exports initialized: status=0
Go backend start requested: /data/storage/el2/base/haps/entry/files
StartServer dataDir=/data/storage/el2/base/haps/entry/files
Go backend start returned: true
```

随后 ArkWeb 初始化、`backend HTTP ready after N retries`、`Web page loaded: http://127.0.0.1:28976/`，页面显示 HomeCast 首页。

> 就绪前 `backend probe` 的 `code:2300007` / `osErr:"111"` / `tcpConnE:"115"` 是**正常轮询失败日志**，不是失败证据；以 `start returned: true` + 页面加载日志为准。

## 本轮根因（必读）：Windows 绝对路径进了 libentry.so 的 DT_NEEDED

**现象**：正式 HAP 内 ArkTS `import { startServer } from 'libentry.so'` 一直报
`SyntaxError: the requested module 'entry' does not provide an export name 'startServer'`，
即 NAPI `Init` 从未执行（`RegisterEntryModule called` 都打不出来 → `libentry.so` 加载失败）。

**错误链**（逐层定位顺序）：

1. `Go backend start failed: {}` → EntryAbility 异常是 `undefined` 时 `JSON.stringify` 得空对象，不可读
2. 改 `String(err)` + `JSON.stringify(err)` 后：`TypeError: Cannot read property startServer of undefined` → 是**默认导入**把模块拿成了 `undefined`
3. 改命名导入后：`SyntaxError: ... does not provide an export name 'startServer'` → **libentry.so 没加载成功**
4. `hvigorw clean` 强制重编 + napi_init.cpp 加 `OH_LOG_Print` 后仍是这个错
5. 拉设备侧 `libentry.so`、`readelf -d`，**根因浮现**：
   - `DT_NEEDED` 含 `C:/Users/a1/homecast/wails3/build/harmony/entry/src/main/cpp/libs/x86_64/libhomecast_go.so`
   - RUNPATH 同样指向 Windows 全路径
   → 设备上当然不存在这个路径，**Go 库加载失败 → 整个 libentry.so 加载失败 → NAPI 注册没发生**。

**修复**（`entry/src/main/cpp/CMakeLists.txt`）：

```cmake
set(CMAKE_SKIP_RPATH TRUE)
add_library(homecast_go SHARED IMPORTED GLOBAL)
set_target_properties(homecast_go PROPERTIES
    IMPORTED_LOCATION ${NATIVERENDER_ROOT_PATH}/libs/${OHOS_ARCH}/libhomecast_go.so
    IMPORTED_NO_SONAME TRUE)
```

修复后 `readelf -d`：NEEDED 只剩 `libhomecast_go.so`，无 RUNPATH、无 Windows 路径。HAP 内只按库名加载。

**为什么此前最小 C 加载器能跑**：那是直接把设备上的 .so 当文件加载（路径给对了），跟正式 HAP 把错误依赖路径打进 `libentry.so` 的 ELF 头不是一回事——所以「能跑」与「HAP 里挂」不矛盾。

## 失败分支汇总

| 现象 | 原因 | 处理 |
|---|---|---|
| `Go backend start failed: {}` | 异常不可读 | ArkTS 日志改 `String(err)` + `JSON.stringify(err)` |
| `Cannot read property startServer of undefined` | Index.d.ts 是**命名导出**，默认导入 = undefined | `import { startServer } from 'libentry.so'` |
| `does not provide an export name 'startServer'` | libentry.so 加载失败（依赖缺失） | readelf 定位 → CMake 修复 |
| 改 CMake 不生效 | hvigor 增量缓存 | `hvigorw clean` 重编 |
| `bind: permission denied`（hdc shell） | shell 环境 ≠ 正式 App 沙盒 | 改用 `this.context.filesDir` 显式传沙盒目录 |
| 首启白屏 | ArkWeb 先于后端就绪加载 | Index.ets 轮询 HTTP 200 后再设 URL（最多 100 次 * 200ms） |

## CI 覆盖与边界（为什么不真跑 HAP 构建）

`.github/workflows/ci.yml` 里有 `harmony-checks`（Linux runner，秒级）：守壳工程关键资产 + 历史根因修复防回归，不真机构建：

- 壳工程关键文件齐全（app.json5 / module.json5 / CMakeLists.txt / napi_init.cpp / Index.d.ts / EntryAbility.ets / Index.ets / network_config.json 等）
- 防回归：CMakeLists 带 `CMAKE_SKIP_RPATH` + `IMPORTED_NO_SONAME`（堵死绝对路径进 DT_NEEDED 的复发）
- 防回归：Index.d.ts 命名导出 ↔ EntryAbility 命名导入对齐；INTERNET 权限 + 明文 HTTP 配置在位
- Go 鸿蒙入口 `main_harmony.go` 存在且 `//export StartServer`
- `build-harmony.md` 是完整复现手册（含步骤/根因/签名）

**为什么 GitHub Actions 跑不了真鸿蒙构建**：HAP 构建依赖 DevEco Studio 生态的闭源/本机组件——hvigor 构建、OpenHarmony SDK（native/llvm clang）、hap-sign-tool 签名（本地调试证书 p12/pem）、Emulator 模拟器。这些在 GitHub runner 上不可用/不可分。**真机构建路径按本文档第 1–5 节在 win11 开发机本机执行**（`build-harmony-x86_64.bat` / `build-harmony-arm64.bat` / `sign-homecast-hap.bat` 已固化）。

## 为 wails3 社区做贡献（复现 + 贡献路径）

HomeCast 鸿蒙壳是**wails3 官方不装机器的第四端参考实现**（wails3 只支持 linux/windows/darwin/android/ios，无鸿蒙）。本仓库坚持不 fork wails3，壳层 ArkTS 自建、业务与前端仍复用 Go 后端 + go:embed——这套「不改 wails3、壳层外挂」的路径本身就可以回输社区：

1. **复现路径公开**：本文档 + `docs/2026-11-08-HomeCast-鸿蒙版从编译签名到模拟器跑通排查记录.md` + `go/cmd/server/main_harmony.go` + `wails3/build/harmony/` 全部公开，任何人照做可复现「c-shared 后端 + NAPI + ArkWeb」链路。
2. **可贡献给 wails3 上游的内容**（供 fork/issue/PR 引用）：
   - `build/harmony/` 整套 ArkTS 壳工程模板（ArkWeb 轮询后端 + NAPI 加载 Go c-shared + 明文 HTTP 配置 + 命名导出契约）
   - `main_harmony.go` 的 `StartServer(dataDir)` 嵌入式入口范式（正式 App 沙盒目录 + 显式 HOME/HC_DATA_DIR，不杀宿主进程）
   - 构建脚本三件套（x86_64/arm64 交叉编译、hap-sign-tool 签名、hvigorw assembleHap）
   - 一个明确的**坑情报**：CMake 交叉链接 IMPORTED 库会把构建机绝对路径写进 DT_NEEDED/RUNPATH，必须 `CMAKE_SKIP_RPATH` + `IMPORTED_NO_SONAME`（wails3 若哪天加鸿蒙，这条对 NDK/OHOS 交叉都适用）
3. **给 wails3 上游的 issue 主题建议**（内容已在本文档/排查记录里，可直接引用）：
   - 请求：wails3 官方对 HarmonyOS/OpenHarmony 的支持态度与路线图（community 是否接入 ohos-go / OpenHarmony SDK 的 c-shared 交叉编译）
   - 附上本仓库已跑通的参考实现链接，避免社区重复踩坑

> 贡献动作（提 issue / 开 PR / fork 演示）按用户节奏执行，本仓库文档与工程资产已就绪。

## 待办边界（已定，不再展开）

- 搜索/播放最小业务链路待验证（不能仅凭首页显示宣称全部通过）
- 歌词/悬浮窗/锁屏仍按 iOS 节奏暂缓
- host 侧 `hdc fport` + 宿主机 curl 28976 有 `Connection was reset` 残留现象，属宿主侧调试问题，不影响 App 内显示成功，后续需要时再查