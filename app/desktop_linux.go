//go:build linux

package main

import (
	"context"
	"log"
	"sync/atomic"

	"github.com/elionaz/panal/internal/desktopnotify"
	"github.com/elionaz/panal/internal/linuxnotify"
	"github.com/elionaz/panal/internal/macnotify"
)

// linuxDesktop: notifications through the freedesktop notification
// service (clicking one opens its session); no badge or bounce. Whether the
// window has the focus comes from the frontend (WindowFocus).
type linuxDesktop struct {
	n   *linuxnotify.Notifier
	err error
}

func newDesktop(_ context.Context, onClick func(string)) desktop {
	if desktopnotify.Disabled() {
		return linuxDesktop{err: linuxnotify.ErrUnavailable}
	}
	n, err := linuxnotify.New("panal", onClick)
	if err != nil {
		log.Printf("panal: notifications: %v", err)
	}
	return linuxDesktop{n: n, err: err}
}

func (d linuxDesktop) Notify(n note) error {
	if d.n == nil {
		return nil
	}
	return d.n.Post(linuxnotify.Note{Key: n.SessionID, Title: n.Title, Body: n.Body, Sound: n.Sound})
}

func (d linuxDesktop) Post(n note) error {
	if d.n == nil {
		return d.err
	}
	return d.n.Post(linuxnotify.Note{Key: n.SessionID, Title: n.Title, Body: n.Body, Sound: n.Sound})
}

func (d linuxDesktop) Withdraw(id string) {
	if d.n != nil {
		d.n.Withdraw(id)
	}
}

func (linuxDesktop) SetBadge(int)      {}
func (linuxDesktop) RequestAttention() {}
func (linuxDesktop) IsActive() bool    { return windowFocused.Load() }
func (linuxDesktop) Activate()         {} // focusSession shows the window

func (d linuxDesktop) Refresh() (macnotify.Status, string) {
	if desktopnotify.Disabled() {
		return macnotify.StatusUnknown, "notifications are off (PANAL_NOTIFY)"
	}
	if d.n == nil {
		return macnotify.StatusUnknown, d.err.Error()
	}
	return macnotify.StatusAuthorized, ""
}

var windowFocused atomic.Bool
