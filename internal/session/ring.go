package session

import "sync"

// DefaultRingSize is the default scrollback capacity per session.
const DefaultRingSize = 8 << 20

// Ring is a fixed-capacity byte buffer that keeps the most recent bytes
// written to it. It is safe for concurrent use.
type Ring struct {
	mu    sync.Mutex
	buf   []byte
	w     int   // next write position
	n     int   // bytes stored (<= cap)
	total int64 // bytes ever written
}

// NewRing returns a Ring holding up to size bytes (DefaultRingSize if <= 0).
func NewRing(size int) *Ring {
	if size <= 0 {
		size = DefaultRingSize
	}
	return &Ring{buf: make([]byte, size)}
}

// Write appends p, overwriting the oldest bytes when full. It never fails.
func (r *Ring) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := len(p)
	r.total += int64(n)
	c := len(r.buf)
	if n >= c {
		copy(r.buf, p[n-c:])
		r.w, r.n = 0, c
		return n, nil
	}
	k := copy(r.buf[r.w:], p)
	if k < n {
		copy(r.buf, p[k:])
	}
	r.w = (r.w + n) % c
	r.n = min(r.n+n, c)
	return n, nil
}

// Snapshot returns a copy of the stored bytes, oldest first.
func (r *Ring) Snapshot() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]byte, r.n)
	start := (r.w - r.n + len(r.buf)) % len(r.buf)
	k := copy(out, r.buf[start:min(start+r.n, len(r.buf))])
	copy(out[k:], r.buf[:r.n-k])
	return out
}

// Len returns the number of stored bytes.
func (r *Ring) Len() int { r.mu.Lock(); defer r.mu.Unlock(); return r.n }

// Cap returns the capacity.
func (r *Ring) Cap() int { return len(r.buf) }

// Total returns the number of bytes ever written.
func (r *Ring) Total() int64 { r.mu.Lock(); defer r.mu.Unlock(); return r.total }

// Tail returns a copy of the last n stored bytes (fewer if not available).
func (r *Ring) Tail(n int) []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	n = max(0, min(n, r.n))
	out := make([]byte, n)
	start := (r.w - n + len(r.buf)) % len(r.buf)
	k := copy(out, r.buf[start:min(start+n, len(r.buf))])
	copy(out[k:], r.buf[:n-k])
	return out
}
