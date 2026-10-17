//go:build !android

package main

// 桌面端（Linux/Windows）歌词挂件启动：多态显示层由 widget/ 提供
// （display_linux.go = GTK3 / display_windows.go = Win32），信号回调由 v3 主循环驱动。
// PlayerCtl 事件桥已退役（widget.go），挂件只读快照 + 本地显示。

import (
	"log"
	"time"

	"homecast/app/widget"
)

func startWidget(snapshot widget.SnapshotProvider, backend string) {
	time.AfterFunc(time.Second, func() {
		if err := widget.Start(snapshot, backend); err != nil {
			log.Printf("[widget] 启动失败: %v", err)
		}
	})
}