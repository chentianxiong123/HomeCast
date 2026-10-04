//go:build !android

package main

// 桌面端（Linux/Windows）歌词挂件启动：多态显示层由 widget/ 提供
// （display_linux.go = GTK3 / display_windows.go = Win32），信号回调由 v3 主循环驱动。
// desktopEventsBridge 类型定义在 main.go（公共文件，安卓编译也引用）。

import (
	"log"
	"time"

	"homecast/app/widget"
)

func startWidget(snapshot widget.SnapshotProvider, ctl widget.PlayerCtl, backend string) {
	time.AfterFunc(time.Second, func() {
		if err := widget.Start(snapshot, ctl, backend); err != nil {
			log.Printf("[widget] 启动失败: %v", err)
		}
	})
}