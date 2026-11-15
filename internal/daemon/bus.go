package daemon

import (
	"sync"

	"github.com/elionaz/agenterm/internal/protocol"
)

// bus fans daemon events out to events.subscribe streams. A subscriber that
// falls behind is disconnected (its channel closed) so it can resubscribe and
// resync with session.list instead of silently missing transitions.
type bus struct {
	mu   sync.Mutex
	subs map[chan protocol.Event]struct{}
}

func newBus() *bus { return &bus{subs: make(map[chan protocol.Event]struct{})} }

func (b *bus) subscribe() (<-chan protocol.Event, func()) {
	ch := make(chan protocol.Event, 256)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
	}
}

func (b *bus) publish(ev protocol.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- ev:
		default:
			delete(b.subs, ch)
			close(ch)
		}
	}
}

func (b *bus) closeAll() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		delete(b.subs, ch)
		close(ch)
	}
}
