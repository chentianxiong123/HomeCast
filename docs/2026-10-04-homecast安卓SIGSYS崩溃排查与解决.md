# homecast 安卓版 Waydroid 上 SIGSYS 崩溃排查与解决（换 SQLite 驱动）

## 背景

homecast 是给爸妈做的个人 B 站音乐播放器（Go 后端 + Web 前端），安卓版是"纯 Go 服务端二进制 + 极简 WebView 壳"（等 Wails v3 转正前的过渡方案）。开发机用 **Waydroid（x86_64，Android 13 API 33）** 当测试环境。

问题是：App 界面能起，但**内嵌的 Go 服务端一打开 SQLite 数据库就崩**——打印完 `listening` 日志后立刻 `SIGSYS: bad system call`。这个坑查了两天，最后靠换 SQLite 驱动根治。

## 现象

```
2026/10/04 03:31:41 homecast-go listening on http://0.0.0.0:28976 ...
SIGSYS: bad system call
PC=0x40d00e m=0 sigcode=1

goroutine 1 [syscall]:
syscall.Syscall(0x6, ...)                    // 0x6 = SYS_lstat (amd64)
modernc.org/libc.X__syscall2(...)
modernc.org/libc._fstatat_kstat(...)
modernc.org/libc.X__fstatat(...)
modernc.org/sqlite/lib._unixFullPathname(...)
```

同一 APK 在 **root 托管服务端时正常**（root 进程没有 seccomp filter）→ 锁定是 App 进程的 **Android seccomp 安全策略**在杀。

## 排查过程

### 1. 定位责任链（不是我们的代码）

```
database/sql（标准库，无责）
  → modernc.org/sqlite（纯 Go 移植的 SQLite，自己不直接发 syscall，无责）
    → modernc.org/libc（musl 移植，真正发 syscall 的一方）
       打开库时解析路径 → SYS_lstat（老式系统调用，x86_64 独有）
          → Android seccomp（合法安全策略，只允许 newfstatat 家族）
```

modernc 的 libc 是 **ccgo 生成代码**（musl 原样翻译），`_fstatat_kstat` 为了兼容老 Linux 内核保留了 `SYS_lstat`/`SYS_stat` 老分支。Android 的 seccomp 白名单不认这些老编号 → 直接 SIGSYS 杀进程。

### 2. 关键发现：arm64（爸妈真机）无辜

查 `ccgo_linux_arm64.go`：**arm64 版没有 SYS_lstat/SYS_stat 老分支**，只有 `SYS_fstat(80)` + `SYS_newfstatat`（都在 Android 白名单内）。所以这是 **x86_64 Android 独有的坑**——真机（arm64）大概率没事，但 waydroid/模拟器（x86_64）必崩。

| 环境 | 架构 | modernc 路径 | 结果 |
|---|---|---|---|
| 爸妈真机 | arm64 | newfstatat/fstat（允许） | 干净 |
| 桌面 Linux | x86_64 | 老/新都有（Linux 不拦） | 干净 |
| Waydroid 容器 | x86_64 | 老 syscall 被 seccomp 拦 | 崩 |

### 3. 想"修"依赖的两条路都堵死

- **Go `-overlay` 构建补丁**（构建期把生成文件里的老 syscall 换掉）：`go build` 直接报错 **"Files beneath GOMODCACHE must not be replaced"**——Go 官方禁止 overlay 替换模块缓存文件。此路不通（还白写了个 patch 脚本）。
- **刷上游 issue**：`gitlab.com/cznic/libc` 上 **#41 早有人报过**（2024-08），2025-04 被维护者 cznic 关闭但**从没修**（git 历史零相关提交，最新版 v1.77.1 仍复现；因为是 ccgo 生成代码，改生成物会被下次生成覆盖，修得动生成器没人修）。GitHub 上没有活跃仓库（`modernc/libc` 404，`cznic/sqlite` 2018 年归档），最后用小号 fork + 大号发 issue 存档到 GitHub。

### 4. 正路：换 SQLite 驱动

候选 **`ncruces/go-sqlite3`**（纯 Go，WASM 虚拟化 SQLite）——文件 IO 走 **Go 标准库 os 包**（Go 自己十几年前就为 Android 修过 stat 问题，标准库在 Android 上只用允许的接口）。

**验证（strace，本地跟踪 = 同一二进制在 Android 里的调用面）**：

```
ncruces 版 DB 打开：openat(...) + newfstatat(AT_FDCWD, ..., AT_SYMLINK_NOFOLLOW)
零个 lstat、零个老 stat —— 全在 Android seccomp 白名单内
```

对照 modernc 版：`syscall.Syscall(0x6)` = lstat，被毙。

## 解决方案

`go/internal/store/db.go` 换驱动，就三处改动：

```go
// 之前
_ "modernc.org/sqlite"
db, err := sql.Open("sqlite", path)

// 之后
_ "github.com/ncruces/go-sqlite3/driver"
db, err := sql.Open("sqlite3", path)
```

验证全过：
- **37 个单测 + `-race` 全绿**
- **数据兼容**：modernc 时代建的旧库 `~/.config/homecast/homecast.db` 直接读出真实收藏（SQLite 文件格式一致）
- **体积**：服务端二进制 14MB → 3.3MB
- **Waydroid 实测**：重出 APK 装进 App，内嵌服务端**存活稳定、零 SIGSYS**，`fav/list` 和页面 HTML 全通，adb forward 直连确认请求打到了 App 进程内的服务端

## 持久化与 App 升级（实测结论）

**WASM 只是"SQLite 怎么跑"，数据照样写真实文件**——引擎通过 WASI → wazero → Go 标准库写到磁盘，WASM 沙箱隔离的是"系统调用方式"不是"存储介质"。安卓侧数据落在 App 私有目录 `filesDir/.config/homecast/homecast.db`（MainActivity 设 HOME=filesDir），Android 保证私有目录持久化（卸载/清数据才丢）。

Waydroid 上跑了三轮实测：

1. **写入**：`POST /api/v1/fav/add` 加一条 `BV1TEST0001` → `fav/list` 立刻可见 ✓
2. **进程重启**：`am force-stop` 杀 App → 重新启动 → 收藏完好 ✓
3. **App 升级**：`adb install -r` 覆盖安装 → 重启 → 收藏完好 ✓（Android 覆盖安装保留私有数据，卸载才清）

**给爸妈升级的直接含义**：以后发新版本直接覆盖安装新 APK 即可，歌单/设置零丢失；只有卸载重装或手动清数据才会回到空库。

## 壳的两处稳定性修复（ETXTBSY + 主线程闪退）

调试期踩了两个壳层坑，都是用户场景必踩的：

1. **ETXTBSY（重复打开）**：App 每次 onCreate 都覆盖写 assets 拷出的 hc-server 二进制并重启——后台实例/重复打开时旧进程还映射着该文件，直写被内核拒（`Text file busy`）。修复：启动前先探测 28976 已就绪则直接复用（单实例语义）；拷贝改「写 tmp → `Files.move(REPLACE_EXISTING)`」，rename 可替换被映射的文件。
2. **主线程网络闪退**：第一版把端口探测放进了 onCreate **主线程**——Android 主线程禁网络（StrictMode 抛 `NetworkOnMainThreadException`，RuntimeException，普通 try-catch 抓不住）→ 一点开就闪退。修复：启动全流程（探测→启动→轮询→load）移入后台线程，主线程零网络，`loadUrl` 回主线程执行。

实测（waydroid）：force-stop 冷启动、后台复用、连开三次均正常，收藏数据完好。

## 坑与教训

1. **`go build -overlay` 不能替换 GOMODCACHE 下的文件**——官方硬限制，报错前会静默忽略导致 patch 根本没进二进制（还以为是 patch 写错了）。
2. **`pkill -f "xxx"` 会匹配命令行自身**——bash 命令行里含那个字符串就把自己杀了，命令输出全空，排查半天才发现。
3. **Waydroid 图形会话要 Wayland/X**——headless shell 里 `waydroid session start` 起不来（dbus-launch 崩 / 找不到 Wayland socket），session 归用户自己开。
4. **刷 issue 前先搜**——`gitlab.com/cznic/libc` #41 撞车（已关未修）；GitHub 侧 `modernc/*` 全 404，唯一存在的是 2018 年归档的 `cznic/sqlite`（fork 后可开 issue，但上游看不见）。
5. **strace 过滤**：`strace -e trace=%stat` 在这版 strace 里没抓到任何调用（过滤集问题），直接全跟踪再 grep 才是稳的。
6. **`adb exec-out screencap -p` 的 stderr 会混进 PNG**——重定向没写干净会把二进制文件搞坏。
7. **Android 主线程禁网络**：StrictMode 直接抛 `NetworkOnMainThreadException`（RuntimeException）——任何 HTTP 探测必须放后台线程，否则一点开就闪退；最坑的是**旧进程活着时探测返回 200，调试永远发现不了**，旧进程一死就必现。
8. **覆盖写正在执行的文件 = ETXTBSY**：可执行文件被进程映射时 `open(O_WRONLY/truncate)` 被内核拒；标准做法是写 `tmp` 再 `rename` 替换（旧 inode 由运行中的进程保留，新文件就位）。

## 相关链接

- GitHub 存档 issue：https://github.com/niubihaizi/sqlite/issues/1
- GitLab 上游原 issue（已关未修）：https://gitlab.com/cznic/libc/-/issues/41
- Go 官方当年修同类问题的记录：https://github.com/golang/go/issues/27797
