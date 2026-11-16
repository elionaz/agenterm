package daemon

import (
	"cmp"

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
	// Read at notify time: a ui.language change applies without restart.
	lang := st.UI.Lang()
	if ev.AttentionKind == "stall" && ev.AttentionMessage == "" {
		ev.AttentionMessage = desktopnotify.StallEventBody(lang, ev)
	}
	s.cfg.Notifier(Notification{
		Title:     desktopnotify.Title(ev.ProjectName, cmp.Or(ev.SessionName, ev.ProfileID)),
		Body:      desktopnotify.Body(lang, ev.AttentionKind, ev.AttentionMessage),
		Sound:     st.Notify.WantsSound(ev.AttentionKind),
		SessionID: ev.SessionID,
		Kind:      ev.AttentionKind,
	})
}

func osascriptNotify(n Notification) {
	_ = desktopnotify.Osascript(n.Title, n.Body, n.Sound)
}
