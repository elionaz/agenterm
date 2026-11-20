package main

import (
	"log"

	"github.com/elionaz/panal/internal/macnotify"
)

// openURL handles panal://session/<id> (options.Mac.OnUrlOpen): what the
// daemon opens when one of its notifications is clicked while the app was
// closed. On a cold launch it arrives before the frontend listens, so the
// id is also kept until the frontend asks for it (TakePendingFocus).
func (a *App) openURL(raw string) {
	id, ok := macnotify.ParseSessionURL(raw)
	if !ok {
		log.Printf("panal: ignoring URL %q", raw)
		return
	}
	a.focusMu.Lock()
	ready, started := a.frontendReady, a.started
	if !ready {
		a.pendingFocus = id
	}
	a.focusMu.Unlock()
	if started {
		a.focusSession(id)
	}
}

// TakePendingFocus returns (once) the session a notification click asked to
// show before the frontend was ready, and marks the frontend ready.
func (a *App) TakePendingFocus() string {
	a.focusMu.Lock()
	defer a.focusMu.Unlock()
	a.frontendReady = true
	id := a.pendingFocus
	a.pendingFocus = ""
	return id
}
