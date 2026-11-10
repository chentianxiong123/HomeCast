package playback

import (
	"database/sql"
	"testing"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
)

func TestPlayQueueAndNext(t *testing.T) {
	p := NewPlaybackState()
	p.Play(Song{BVID: "A", Title: "a"})
	p.Play(Song{BVID: "B", Title: "b"})
	s := p.Snapshot()
	if len(s.Queue) != 2 || s.QIdx != 1 || s.BVID != "B" {
		t.Fatalf("Play queue wrong: q=%d idx=%d cur=%s", len(s.Queue), s.QIdx, s.BVID)
	}
	// 去重：再播 A → 移尾
	p.Play(Song{BVID: "A", Title: "a2"})
	s = p.Snapshot()
	if len(s.Queue) != 2 || s.QIdx != 1 || s.BVID != "A" || s.Queue[0].BVID != "B" {
		t.Fatalf("dedupe wrong: q=%d idx=%d cur=%s first=%s", len(s.Queue), s.QIdx, s.BVID, s.Queue[0].BVID)
	}
	// next 循环
	p.Next()
	s = p.Snapshot()
	if s.BVID != "B" {
		t.Fatalf("next wrong: %s", s.BVID)
	}
	p.Next()
	s = p.Snapshot()
	if s.BVID != "A" {
		t.Fatalf("next loop wrong: %s", s.BVID)
	}
}

func TestToggleSeekMode(t *testing.T) {
	p := NewPlaybackState()
	p.Play(Song{BVID: "A"})
	p.Toggle()
	if p.Snapshot().Playing {
		t.Fatal("toggle should pause")
	}
	p.Resume()
	if !p.Snapshot().Playing {
		t.Fatal("resume should play")
	}
	p.Seek(42)
	if p.Snapshot().Position != 42 {
		t.Fatalf("seek: %v", p.Snapshot().Position)
	}
	p.SetMode("random")
	if p.Snapshot().PlayMode != "random" {
		t.Fatal("mode")
	}
}

func TestSwitchSource(t *testing.T) {
	p := NewPlaybackState()
	p.Play(Song{BVID: "A"})
	p.SwitchSpeaker("did123")
	s := p.Snapshot()
	if s.Source != "speaker" || s.SpkDID != "did123" {
		t.Fatalf("switch speaker: %+v", s)
	}
	p.SwitchWeb()
	s = p.Snapshot()
	if s.Source != "web" || s.SpkDID != "" {
		t.Fatalf("switch web: %+v", s)
	}
}

func TestRemoveClearQueue(t *testing.T) {
	p := NewPlaybackState()
	p.SetQueue([]Song{{BVID: "A"}, {BVID: "B"}, {BVID: "C"}}, 0)
	if p.Snapshot().QIdx != 0 || p.Snapshot().BVID != "A" {
		t.Fatalf("setqueue: %+v", p.Snapshot())
	}
	p.RemoveQueue("A")
	s := p.Snapshot()
	if len(s.Queue) != 2 || s.BVID != "B" {
		t.Fatalf("remove current wrong: q=%d cur=%s", len(s.Queue), s.BVID)
	}
	p.ClearQueue()
	s = p.Snapshot()
	if len(s.Queue) != 0 || s.BVID != "" {
		t.Fatalf("clear: q=%d bvid=%s", len(s.Queue), s.BVID)
	}
}

func TestSubscribeBroadcast(t *testing.T) {
	p := NewPlaybackState()
	ch := p.Subscribe()
	defer p.Unsubscribe(ch)
	p.Play(Song{BVID: "A"})
	select {
	case m := <-ch:
		if m.Name != "state" {
			t.Fatalf("first event should be state, got %s", m.Name)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast")
	}
	// progress 事件
	p.SetProgress(10, 100, true)
	select {
	case m := <-ch:
		if m.Name != "progress" {
			t.Fatalf("second event should be progress, got %s", m.Name)
		}
	case <-time.After(time.Second):
		t.Fatal("no progress broadcast")
	}
}

func TestNextBVID(t *testing.T) {
	p := NewPlaybackState()
	p.SetQueue([]Song{{BVID: "A"}, {BVID: "B"}}, 0)
	if p.NextBVID() != "B" {
		t.Fatal("next bvid")
	}
	p.Next()
	if p.NextBVID() != "A" {
		t.Fatal("next bvid loop")
	}
}

func TestSetSpeakerProgress(t *testing.T) {
	p := NewPlaybackState()
	p.Play(Song{BVID: "A", Title: "songA"})
	// 未切音箱：progress 不同步状态（source 仍 web）
	p.SetSpeakerProgress("did1", "A", "songA", 10, 200, true)
	s := p.Snapshot()
	if s.Source != "web" || s.Position != 0 {
		t.Fatalf("should not sync before switch: %+v", s)
	}
	// 切音箱后：progress 同步
	p.SwitchSpeaker("did1")
	p.SetSpeakerProgress("did1", "A", "songA", 30, 200, true)
	s = p.Snapshot()
	if s.Source != "speaker" || s.Position != 30 || s.Duration != 200 || !s.Playing {
		t.Fatalf("speaker progress not synced: %+v", s)
	}
	// 其他 did 不同步
	p.SetSpeakerProgress("did2", "B", "songB", 99, 300, true)
	s = p.Snapshot()
	if s.Position != 30 || s.BVID != "A" {
		t.Fatalf("other did should not sync: %+v", s)
	}
	// 播完
	p.SetSpeakerStopped("did1")
	s = p.Snapshot()
	if s.Playing || s.Position != 0 {
		t.Fatalf("stopped wrong: %+v", s)
	}
}

func TestPersistRestore(t *testing.T) {
	// 临时 SQLite
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT)`); err != nil {
		t.Fatal(err)
	}
	p := NewPlaybackState()
	p.SetDB(db)
	p.SetQueue([]Song{{BVID: "A", Title: "a"}, {BVID: "B", Title: "b"}}, 1)
	p.SetMode("random")
	p.Save(db) // 主动触发（SetQueue/SetMode 经 bcastSnapshot 已自动存，此处显式保底）
	// 新实例恢复
	p2 := NewPlaybackState()
	p2.Restore(db)
	s := p2.Snapshot()
	if len(s.Queue) != 2 || s.QIdx != 1 || s.BVID != "B" || s.PlayMode != "random" {
		t.Fatalf("restore wrong: queue=%d qidx=%d cur=%s mode=%s", len(s.Queue), s.QIdx, s.BVID, s.PlayMode)
	}
	if s.Playing {
		t.Fatal("restore should not auto-play")
	}
}
