package main

import (
	"bytes"
	"sync"
	"testing"
	"time"
)

type sink struct {
	mu     sync.Mutex
	chunks [][]byte
}

func (s *sink) emit(b []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chunks = append(s.chunks, b)
}

func (s *sink) snapshot() [][]byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([][]byte(nil), s.chunks...)
}

func TestCoalescerBatchesSmallWrites(t *testing.T) {
	var s sink
	c := newCoalescer(20*time.Millisecond, 1024, s.emit)
	for range 50 {
		c.Write([]byte("ab"))
	}
	if n := len(s.snapshot()); n != 0 {
		t.Fatalf("emitted %d chunks before the delay", n)
	}
	time.Sleep(80 * time.Millisecond)
	got := s.snapshot()
	if len(got) != 1 || !bytes.Equal(got[0], bytes.Repeat([]byte("ab"), 50)) {
		t.Fatalf("want one 100-byte chunk, got %d chunks", len(got))
	}
}

func TestCoalescerSplitsAtMaxBytes(t *testing.T) {
	var s sink
	c := newCoalescer(time.Hour, 4, s.emit)
	c.Write([]byte("0123456789"))
	got := s.snapshot()
	if len(got) != 2 || string(got[0]) != "0123" || string(got[1]) != "4567" {
		t.Fatalf("unexpected chunks %q", got)
	}
	c.Flush()
	got = s.snapshot()
	if len(got) != 3 || string(got[2]) != "89" {
		t.Fatalf("flush: unexpected chunks %q", got)
	}
}

func TestCoalescerFlushIsImmediateAndOrdered(t *testing.T) {
	var s sink
	c := newCoalescer(time.Hour, 1<<20, s.emit)
	c.Write([]byte("a"))
	c.Flush()
	c.Write([]byte("b"))
	c.Flush()
	c.Flush() // nothing pending: no empty emit
	got := s.snapshot()
	if len(got) != 2 || string(got[0]) != "a" || string(got[1]) != "b" {
		t.Fatalf("unexpected chunks %q", got)
	}
}

func TestCoalescerStopDiscards(t *testing.T) {
	var s sink
	c := newCoalescer(5*time.Millisecond, 1024, s.emit)
	c.Write([]byte("pending"))
	c.Stop()
	c.Write([]byte("after"))
	c.Flush()
	time.Sleep(30 * time.Millisecond)
	if got := s.snapshot(); len(got) != 0 {
		t.Fatalf("emitted after Stop: %q", got)
	}
}

func TestCoalescerConcurrentWritesKeepAllBytes(t *testing.T) {
	var s sink
	c := newCoalescer(time.Millisecond, 64, s.emit)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 200 {
				c.Write([]byte("xyz"))
			}
		}()
	}
	wg.Wait()
	c.Flush()
	total := 0
	for _, ch := range s.snapshot() {
		if len(ch) > 64 {
			t.Fatalf("chunk of %d bytes exceeds max", len(ch))
		}
		total += len(ch)
	}
	if total != 8*200*3 {
		t.Fatalf("got %d bytes, want %d", total, 8*200*3)
	}
}
