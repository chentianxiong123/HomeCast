# desktop/ — 🐧 Linux 桌面壳（唯一带挂件的端）

**这个目录是什么**：HomeCast 的 Linux 桌面客户端。wails3 v3 壳（WebView 窗口）+ **GTK 桌面歌词挂件**。运行在用户 Linux 桌面上（主窗口 + 悬浮歌词双窗口）。

**为什么它单独占一个目录（而不在 wails3/）**：
- 挂件 = cgo（GTK 是 C 库，Go 调 C 库只有 cgo 一条路，编译要本机 gtk 开发库）
- 带 cgo 的壳没法交叉编译 → 只能本机 Linux 构建 → 和 wails3/（纯 Go 交叉编 APK/exe）分开
- 挂件本身是多态的（`widget/`），Win32 版代码在 `widget/display_windows.go`，真机验证后也能编 Windows 挂件

## 构建

```bash
cd desktop && ./build.sh        # 产 homecast-desktop（含 GTK 挂件）
```

## 挂件多态层（widget/）

```
widget/
├── display.go          # Display/Canvas 接口（多态，三端共享）
├── display_linux.go    # GTK3 实装（cgo，本目录 Linux 端在用）
├── display_windows.go  # Win32 实装（cgo，Windows 挂件，待真机验证）
├── widget.go           # 纯核心（tick/绘制/交互，三端共享）
└── cglue.c             # C 胶水（#ifndef _WIN32 保护，仅 Linux 编译）
```

**windemo/**：Windows 挂件最小 demo（`GOOS=windows CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc go build -o windemo.exe ./windemo/`，Wine 里验证 GUI 用）。

## 已知坑

- Wine + WebView2：渲染区内鼠标光标不显示（WineHQ 58922 未修复，真 Windows 正常）
- Wine 的 X11 后端对 Layered 透明窗口支持有限 → Windows 挂件透明效果要在真 Windows 验证