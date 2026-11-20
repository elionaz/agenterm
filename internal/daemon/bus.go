package daemon

import (
	"sync"

	"github.com/elionaz/panal/internal/protocol"
)

// bus fans daemon events out to events.subscribe streams. A subscriber that
// falls behind is disconnected (its channel closed) so it can resubscribe and
// resync with session.list instead of silently missing transitions.
type bus struct {
	mu   sync.Mutex
	subs map[chan protocol.Event]bool // value: subscriber is a UI client
}

func newBus() *bus { return &bus{subs: make(map[chan protocol.Event]bool)} }

// subscribe registers a stream; ui marks desktop apps (see uiCount).
func (b *bus) subscribe(ui bool) (<-chan protocol.Event, func()) {
	ch := make(chan protocol.Event, 256)
	b.mu.Lock()
	b.subs[ch] = ui
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

// uiCount is the number of subscribed UI clients.
func (b *bus) uiCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	for _, ui := range b.subs {
		if ui {
			n++
		}
	}
	return n
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
