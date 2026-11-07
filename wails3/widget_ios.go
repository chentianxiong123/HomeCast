//go:build ios

package main

// iOS 端挂件 no-op：iOS 无桌面歌词挂件（系统禁止跨 App 悬浮窗），纯 App 内播放器。
// 挂件包（cgo/AppKit/GTK/Win32）不参与 iOS 编译。

func startWidget(_ any, _ string) {}