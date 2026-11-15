package main

import (
	"context"
	"log"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/elionaz/agenterm/internal/desktopnotify"
	"github.com/elionaz/agenterm/internal/protocol"
	"github.com/elionaz/agenterm/internal/settings"
)

// evFocusSession asks the frontend to select a session (notification
// clicked).
const evFocusSession = "app:focus-session"

// note is a notification to show.
type note struct {
	SessionID, Kind, Title, Body string
	Sound                        bool
}

// desktop is the OS integration: notifications, Dock badge and bounce.
type desktop interface {
	Notify(n note) error
	Withdraw(sessionID string) // remove a session's delivered notification
	SetBadge(n int)
	RequestAttention() // bounce the Dock icon once
	IsActive() bool    // the app is frontmost
	Activate()
}

// nopDesktop is used until startup wires the real one (and in tests).
type nopDesktop struct{}

func (nopDesktop) Notify(note) error { return nil }
func (nopDesktop) Withdraw(string)   {}
func (nopDesktop) SetBadge(int)      {}
func (nopDesktop) RequestAttention() {}
func (nopDesktop) IsActive() bool    { return false }
func (nopDesktop) Activate()         {}

// attention tracks which sessions need the user, to drive notifications
// and the Dock badge.
type attention struct {
	mu      sync.Mutex
	unread  map[string]bool
	visible string // session shown in the window (SetVisibleSession)
	badge   int
}

// sync replaces the unread set from a session list (after a resync).
func (a *App) syncAttention(list []protocol.SessionInfo) {
	a.att.mu.Lock()
	a.att.unread = make(map[string]bool, len(list))
	for _, s := range list {
		if s.Unread {
			a.att.unread[s.ID] = true
		}
	}
	a.att.mu.Unlock()
	a.updateBadge()
}

// onDaemonEvent updates the unread set and notifies on raised attention.
func (a *App) onDaemonEvent(e protocol.Event) {
	switch e.Type {
	case protocol.EventSessionState, protocol.EventSessionAttention:
		a.att.mu.Lock()
		was := a.att.unread[e.SessionID]
		if e.Unread {
			a.att.unread[e.SessionID] = true
		} else {
			delete(a.att.unread, e.SessionID)
		}
		a.att.mu.Unlock()
		if was && !e.Unread {
			a.desk.Withdraw(e.SessionID)
		}
	case protocol.EventSessionActivity:
		// A stall is a quiet heads-up, not an attention: no badge, no
		// unread, no sound (WantsSound is false for "stall").
		if e.StallRaised {
			e.AttentionKind = "stall"
			e.AttentionMessage = desktopnotify.StallBody(e.Activity, e.LastOutputAt, e.LastEventAt, e.StalledAt)
			a.notify(e)
		}
		return
	case protocol.EventSessionRemoved:
		a.att.mu.Lock()
		delete(a.att.unread, e.SessionID)
		a.att.mu.Unlock()
		a.desk.Withdraw(e.SessionID)
	default:
		return
	}
	a.updateBadge()
	if e.Raised && e.Unread {
		a.notify(e)
	}
}

func (a *App) updateBadge() {
	a.att.mu.Lock()
	n := len(a.att.unread)
	changed := n != a.att.badge
	a.att.badge = n
	a.att.mu.Unlock()
	if changed {
		a.desk.SetBadge(n)
	}
}

// notify shows a notification unless the user is already looking at that
// session (the frontend acks it then).
func (a *App) notify(e protocol.Event) {
	st, err := settings.Load(a.paths.Settings())
	if err != nil {
		log.Printf("agenterm: settings: %v", err)
	}
	active := a.desk.IsActive()
	a.att.mu.Lock()
	visible := a.att.visible == e.SessionID
	a.att.mu.Unlock()
	if active && visible {
		return
	}
	if !st.Notify.ShouldNotify(e.AttentionKind) {
		return
	}
	n := note{
		SessionID: e.SessionID,
		Kind:      e.AttentionKind,
		Title:     desktopnotify.Title(e.ProjectName, e.ProfileID),
		Body:      desktopnotify.Body(e.AttentionKind, e.AttentionMessage),
		Sound:     st.Notify.WantsSound(e.AttentionKind),
	}
	go func() {
		if err := a.desk.Notify(n); err != nil {
			log.Printf("agenterm: notify: %v", err)
		}
	}()
	if e.AttentionKind == "permission" && !active {
		a.desk.RequestAttention()
	}
}

// focusSession is called when a notification is clicked.
func (a *App) focusSession(id string) {
	a.desk.Activate()
	if a.wailsCtx != nil {
		runtime.WindowUnminimise(a.wailsCtx)
		runtime.WindowShow(a.wailsCtx)
	}
	a.emit(evFocusSession, id)
}

// SetVisibleSession tells the backend which session the window shows, so
// it does not notify about what the user is looking at.
func (a *App) SetVisibleSession(id string) {
	a.att.mu.Lock()
	a.att.visible = id
	a.att.mu.Unlock()
}

// Ack marks a session's attention as seen (selected / focused).
func (a *App) Ack(id string) error {
	return a.call(func(ctx context.Context) error {
		_, err := a.client.Ack(ctx, id)
		return err
	})
}
