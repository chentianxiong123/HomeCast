//go:build windows

// windemo：Windows 桌面歌词挂件最小演示（Wine/真机跑）
// 起内嵌后端 + 模拟播放（晴天·周杰伦）→ 挂件窗口置顶穿透显示歌词滚动。
// 交叉编译：GOOS=windows CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc go build -o windemo.exe ./windemo/
package main

import (
	"log"
	"math"
	"os"
	"time"

	"homecast/server"
	"homecast-desktop/widget"
)

type demoEvents struct{}

func (demoEvents) Emit(cmd string, payload ...any) {
	log.Printf("[demo] emit %s %v", cmd, payload)
}

func main() {
	port := os.Getenv("HC_PORT")
	if port == "" {
		port = server.Port
	}
	// 内嵌后端（挂件拉歌词用；起不来就只显示标题）
	go func() {
		if err := server.ListenAndServe("127.0.0.1:" + port); err != nil {
			log.Printf("[backend] %v", err)
		}
	}()
	time.Sleep(600 * time.Millisecond)

	// 模拟播放：晴天（周杰伦），进度循环 300s
	t0 := time.Now()
	snap := func() (bvid, title, artist string, ct, dur float64, playing bool) {
		el := time.Since(t0).Seconds()
		return "BV1Qx411w7KC", "晴天", "周杰伦", math.Mod(el, 300), 300, true
	}
	if err := widget.Start(snap, demoEvents{}, "http://127.0.0.1:"+port); err != nil {
		log.Fatalf("[widget] 启动失败: %v", err)
	}
	log.Println("[demo] 挂件已启动，窗口置顶穿透显示歌词")
	widget.MainLoop()
	log.Println("[demo] 挂件已退出")
}