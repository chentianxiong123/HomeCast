//go:build android

package main

// 安卓端挂件 no-op：系统级悬浮窗歌词（TYPE_APPLICATION_OVERLAY + 授权引导）待写，
// 届时把 startWidget 替换为悬浮窗 Service 启动。挂件包（cgo/GTK/Win32）不参与安卓编译。

func startWidget(_ any, _ any, _ string) {}