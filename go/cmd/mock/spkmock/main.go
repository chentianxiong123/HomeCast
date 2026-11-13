// spkmock —— 假音箱：模拟音箱断点续传拉流行为，用来测"流稳不稳"。
//
// 用法（任意音频 URL 都行，本地文件 / hcserver stream 端点 / 网页静态 均可）：
//
//	spkmock -url 'http://127.0.0.1:28976/api/v1/music/stream/BVxxxx?quality=30280&did=108654953'
//	spkmock -url 'file:///path/to/a.m4s'
//	spkmock -url 'http://127.0.0.1:39001/audio.m4s' -chunk 6KB -gap 0.25s
//
// 行为完全复刻音箱（对照真实音箱日志）：
//   - 第一次 GET 无 Range（或 bytes=0-），拉一片，服务器 200/206
//   - 播完这片（gap 时间）→ 下一次 GET 带 Range: bytes=<累计>-
//   - 收到 206 有限片：继续在累计偏移上加；收到 200 全量：按拿到的长度推进
//   - 收到 416/503/空：记一次"断流"，等 retry 后重试（断源/播完场景）
//
// 命令（stdin 一行一个，回车即发）：
//   seek N     跳到第 N 秒（按 bitrate 算偏移，默认 128kbps）重新开始拉
//   pause      停止拉流（模拟断源/暂停，连接全关）
//   resume     恢复拉流（重新 GET）
//   quit       退出并打印汇总
//
// 汇总字段：总拉取字节 / 请求数 / 断流次数 / 重复片次数 / 跳片次数 / 平均片耗时。
// =稳定判定=：无断流、无重复片、无跳片、offset 单调递增 → 流稳定。
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	url      = flag.String("url", "", "音频 URL（http(s):// 或 file://）")
	chunk    = flag.String("chunk", "100KB", "每片期望大小（6KB/100KB/1MB…）")
	gap      = flag.String("gap", "0s", "每片之间等待（模拟播放耗时，0=尽快）")
	bitrate  = flag.Int("bitrate", 128, "kbps，seek 秒→字节换算用")
	retry    = flag.Duration("retry", 500*time.Millisecond, "断流后重试间隔")
	maxChunk int64
	gapDur   time.Duration
)

type stats struct {
	mu            sync.Mutex
	requests      int
	totalBytes    int64
	breaks        int // 断流（416/503/空响应）
	dups          int // 重复片（拿到的 offset < 已累计）
	skips         int // 跳片（拿到的 offset > 已累计）
	now           int64
	expectNext    int64 // 下一个应到的 offset（按 Chr 推进）
	lastRespTotal int64 // 服务器声明的文件总大小（Content-Range 尾）
}

func main() {
	flag.Parse()
	if *url == "" {
		fmt.Println("用法: spkmock -url <audio-url> [-chunk 100KB] [-gap 0s] [-bitrate 128]")
		os.Exit(2)
	}
	var err error
	maxChunk, err = parseSize(*chunk)
	if err != nil {
		fmt.Println("chunk 解析失败:", err)
		os.Exit(2)
	}
	gapDur, _ = time.ParseDuration(*gap)

	ctl := make(chan string)
	go readCommands(ctl)

	client := &http.Client{Timeout: 30 * time.Second}
	st := &stats{}

	var paused bool
	// seek 目标（秒 → 字节），0 = 从头
	seekTo := int64(-1) // -1 = 无 seek 命令

	fmt.Printf("[spkmock] url=%s chunk=%d gap=%s bitrate=%dkbps\n", *url, maxChunk, gapDur, *bitrate)

loop:
	for {
		select {
		case cmd, ok := <-ctl:
			if !ok {
				// stdin 关闭≠退出（后台运行/管道场景），继续拉流；靠 quit/Ctrl-C 退
				ctl = nil
				continue
			}
			parts := strings.Fields(cmd)
			switch parts[0] {
			case "seek":
				if len(parts) > 1 {
					if sec, e := strconv.ParseFloat(parts[1], 64); e == nil {
						bps := int64(*bitrate * 1000 / 8)
						seekTo = int64(bps * int64(sec))
						fmt.Printf("[spkmock] seek %.1fs → offset=%d\n", sec, seekTo)
					}
				}
			case "pause":
				paused = true
				fmt.Println("[spkmock] paused（不再拉流，模拟断源）")
			case "resume":
				paused = false
				fmt.Println("[spkmock] resumed（重新拉流）")
			case "quit":
				break loop
			default:
				fmt.Println("[spkmock] 未知命令:", cmd, "（可用 seek N / pause / resume / quit）")
			}
		default:
			if paused {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			// 一次拉一片
			ok := pullOnce(client, st, &seekTo)
			if !ok {
				time.Sleep(*retry)
			} else if gapDur > 0 {
				time.Sleep(gapDur)
			}
		}
	}
	printSummary(st)
}

// pullOnce 拉一片。返回 false=断流/失败。
func pullOnce(client *http.Client, st *stats, seekTo *int64) bool {
	st.mu.Lock()
	offset := st.now
	if *seekTo >= 0 && *seekTo != offset { // 有 seek 未生效：先重置
		offset = *seekTo
		st.expectNext = offset
	}
	st.mu.Unlock()

	req, _ := http.NewRequest(http.MethodGet, *url, nil)
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	req.Header.Set("User-Agent", "xiaoai") // 音箱 UA
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		st.mu.Lock()
		st.breaks++
		st.mu.Unlock()
		fmt.Printf("  [ERR] offset=%d 请求失败: %v\n", offset, err)
		return false
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	cr := resp.Header.Get("Content-Range")
	cl := resp.Header.Get("Content-Length")

	switch {
	case resp.StatusCode == http.StatusOK:
		// 200 全量：可能是无 Range/服务器忽略 Range
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxChunk+1))
		n := int64(len(body))
		if n > maxChunk { // 服务器给超了 → 我们只吃一片的量
			n = maxChunk
		}
		report(st, offset, 0, n, resp.StatusCode, ct, cr, cl, time.Since(start))
		return true
	case resp.StatusCode == http.StatusPartialContent:
		startOff, endOff, total, ok := parseContentRange(cr)
		if !ok {
			st.mu.Lock()
			st.breaks++
			st.mu.Unlock()
			fmt.Printf("  [ERR] 206 但 Content-Range 不可解析: %q\n", cr)
			return false
		}
		limit := endOff - startOff + 1
		if limit > maxChunk {
			limit = maxChunk
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, limit))
		n := int64(len(body))
		report(st, startOff, total, n, resp.StatusCode, ct, cr, cl, time.Since(start))
		return true
	default:
		st.mu.Lock()
		st.breaks++
		st.mu.Unlock()
		fmt.Printf("  [BRK] offset=%d status=%d body优先片段=%q\n", offset, resp.StatusCode, "")
		io.Copy(io.Discard, io.LimitReader(resp.Body, 128))
		return false
	}
}

// report 记录一片并推进累计偏移 / 判定重复片/跳片
func report(st *stats, startOff, total, n int64, code int, ct, cr, cl string, dur time.Duration) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.requests++
	if total > 0 {
		st.lastRespTotal = total
	}
	if n == 0 {
		st.breaks++
		fmt.Printf("  [BRK] offset=%d 空响应(code=%d) cr=%s\n", startOff, code, cr)
		return
	}
	if startOff < st.expectNext {
		st.dups++
	}
	if startOff > st.expectNext {
		st.skips++
	}
	st.totalBytes += n
	st.now = startOff + n
	st.expectNext = st.now
	fmt.Printf("  [OK]  off=%d +%d (总=%d/%d) code=%d ct=%s cr=%q cl=%s %s\n",
		startOff, n, st.now, total, code, ct, cr, cl, dur.Round(time.Millisecond))
}

// parseContentRange 解析 "bytes start-end/total"
func parseContentRange(cr string) (start, end, total int64, ok bool) {
	head := strings.TrimPrefix(cr, "bytes ")
	spec := head
	slash := strings.IndexByte(spec, '/')
	if slash < 0 {
		return
	}
	total, err1 := strconv.ParseInt(spec[slash+1:], 10, 64)
	dash := strings.IndexByte(spec[:slash], '-')
	if dash < 0 {
		return
	}
	start, err2 := strconv.ParseInt(spec[:dash], 10, 64)
	end, err3 := strconv.ParseInt(spec[dash+1:slash], 10, 64)
	return start, end, total, err1 == nil && err2 == nil && err3 == nil
}

// parseSize 解析 6KB / 100KB / 1MB / 1024
func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	mult := int64(1)
	su := strings.ToUpper(s)
	switch {
	case strings.HasSuffix(su, "KB"):
		mult = 1024
		s = s[:len(s)-2]
	case strings.HasSuffix(su, "MB"):
		mult = 1024 * 1024
		s = s[:len(s)-2]
	case strings.HasSuffix(su, "B"):
		s = s[:len(s)-1]
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, err
	}
	return int64(n * float64(mult)), nil
}

func readCommands(ctl chan<- string) {
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		ctl <- line
	}
	// stdin EOF：不 close(ctl)，保持拉流（只 quit 命令/信号才退）
}

func printSummary(st *stats) {
	st.mu.Lock()
	defer st.mu.Unlock()
	fmt.Println("\n===== 汇总 =====")
	fmt.Printf("请求数: %d\n", st.requests)
	fmt.Printf("总拉取: %d 字节 (%s)\n", st.totalBytes, human(st.totalBytes))
	fmt.Printf("断流: %d 次\n", st.breaks)
	fmt.Printf("重复片: %d 次（offset 回退=服务器重复给）\n", st.dups)
	fmt.Printf("跳片: %d 次（offset 前进超过预期=丢数据）\n", st.skips)
	fmt.Printf("声明总量: %d (%s)\n", st.lastRespTotal, human(st.lastRespTotal))
	verdict := "✅ 流稳定"
	if st.breaks > 0 || st.dups > 0 || st.skips > 0 {
		verdict = "❌ 流不稳定"
	}
	fmt.Println("判定:", verdict)
}

func human(n int64) string {
	if n >= 1024*1024 {
		return fmt.Sprintf("%.1fMB", float64(n)/1024/1024)
	}
	if n >= 1024 {
		return fmt.Sprintf("%.1fKB", float64(n)/1024)
	}
	return fmt.Sprintf("%dB", n)
}