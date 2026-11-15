package daemon

import (
	"github.com/elionaz/agenterm/internal/desktopnotify"
	"github.com/elionaz/agenterm/internal/protocol"
	"github.com/elionaz/agenterm/internal/settings"
)

// fallbackNotify is used while no desktop app is subscribed, so attention
// is not lost when the app is closed. It runs asynchronously: it is called
// with the session's status lock held.
func (s *Server) fallbackNotify(ev protocol.Event) {
	go s.notifyNow(ev)
}

func (s *Server) notifyNow(ev protocol.Event) {
	st, err := settings.Load(s.cfg.Paths.Settings())
	if err != nil {
		s.log.Warn("settings", "err", err)
	}
	if !st.Notify.ShouldNotify(ev.AttentionKind) {
		return
	}
	s.cfg.Notifier(Notification{
		Title:     desktopnotify.Title(ev.ProjectName, ev.ProfileID),
		Body:      desktopnotify.Body(ev.AttentionKind, ev.AttentionMessage),
		Sound:     st.Notify.WantsSound(ev.AttentionKind),
		SessionID: ev.SessionID,
		Kind:      ev.AttentionKind,
	})
}

func osascriptNotify(n Notification) {
	_ = desktopnotify.Osascript(n.Title, n.Body, n.Sound)
}
