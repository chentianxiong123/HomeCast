package playback

import (
	"testing"
	"time"
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
	if len(s.Queue) != 0 {
		t.Fatalf("clear: %d", len(s.Queue))
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
