package main

import (
	"sync"
	"time"
)

// coalescer batches PTY output so the webview receives at most one event
// per delay window (or per maxBytes), instead of one per PTY read. emit is
// always called with the lock held, so calls never overlap and keep order.
type coalescer struct {
	delay    time.Duration
	maxBytes int
	emit     func([]byte)

	mu      sync.Mutex
	buf     []byte
	timer   *time.Timer
	stopped bool
}

func newCoalescer(delay time.Duration, maxBytes int, emit func([]byte)) *coalescer {
	return &coalescer{delay: delay, maxBytes: maxBytes, emit: emit}
}

// Write queues p. Full maxBytes chunks are emitted immediately; the
// remainder is emitted when the delay timer fires or on Flush.
func (c *coalescer) Write(p []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return
	}
	c.buf = append(c.buf, p...)
	for len(c.buf) >= c.maxBytes {
		c.emitLocked(c.maxBytes)
	}
	if len(c.buf) == 0 {
		c.stopTimerLocked()
		return
	}
	if c.timer == nil {
		c.timer = time.AfterFunc(c.delay, c.onTimer)
	}
}

// Flush emits whatever is pending right away.
func (c *coalescer) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopTimerLocked()
	if !c.stopped && len(c.buf) > 0 {
		c.emitLocked(len(c.buf))
	}
}

// Stop discards pending data; no emit happens after Stop returns.
func (c *coalescer) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopped = true
	c.stopTimerLocked()
	c.buf = nil
}

func (c *coalescer) onTimer() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.timer = nil
	if !c.stopped && len(c.buf) > 0 {
		c.emitLocked(len(c.buf))
	}
}

func (c *coalescer) stopTimerLocked() {
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
}

// emitLocked emits the first n bytes as a fresh slice (emit may retain it).
func (c *coalescer) emitLocked(n int) {
	out := make([]byte, n)
	copy(out, c.buf[:n])
	c.buf = append(c.buf[:0], c.buf[n:]...)
	c.emit(out)
}
