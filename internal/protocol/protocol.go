// Package protocol defines the wire format between the agenterm daemon and its
// clients.
//
// Control traffic is newline-delimited JSON: the client writes one Request
// per line and the daemon answers with one Response per line. A connection may
// carry any number of sequential control requests.
//
// Two methods take over the connection after their (successful) response:
//
//   - events.subscribe: the daemon streams Event values, one JSON object per line.
//   - session.attach: both sides switch to binary frames (see frame.go).
package protocol

import (
	"encoding/json"
	"time"

	"github.com/elionaz/agenterm/internal/profile"
	"github.com/elionaz/agenterm/internal/project"
)

// Environment variables injected into every session.
const (
	EnvSession = "AGENTERM_SESSION"
	EnvSock    = "AGENTERM_SOCK"
)

// Method names.
const (
	MethodProjectAdd      = "project.add"
	MethodProjectList     = "project.list"
	MethodProjectRemove   = "project.remove"
	MethodSessionSpawn    = "session.spawn"
	MethodSessionList     = "session.list"
	MethodSessionKill     = "session.kill"
	MethodSessionResize   = "session.resize"
	MethodSessionNotify   = "session.notify"
	MethodSessionAck      = "session.ack"
	MethodSessionAttach   = "session.attach"
	MethodSessionTail     = "session.tail"
	MethodProfileList     = "profile.list"
	MethodProfileCatalog  = "profile.catalog"
	MethodProfileSave     = "profile.save"
	MethodProfileDelete   = "profile.delete"
	MethodProfileReorder  = "profile.reorder"
	MethodProfileRescan   = "profile.rescan"
	MethodSettingsGet     = "settings.get"
	MethodSettingsSave    = "settings.save"
	MethodEventsSubscribe = "events.subscribe"
	MethodDaemonPing      = "daemon.ping"
	MethodDaemonShutdown  = "daemon.shutdown"
)

// Request is a single control call.
type Request struct {
	ID     uint64          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response answers the Request with the same ID. Exactly one of Result or
// Error is meaningful.
type Response struct {
	ID     uint64          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// Params and results.

type ProjectAddParams struct {
	Path string `json:"path"`
	Name string `json:"name,omitempty"`
}

type ProjectRef struct {
	ID string `json:"id"`
}

type ProjectList struct {
	Projects []project.Project `json:"projects"`
}

// ProfileList is the result of profile.list/save/delete/reorder/rescan:
// every effective profile (hidden ones included), in menu order.
type ProfileList struct {
	Profiles []profile.Profile `json:"profiles"`
}

// CatalogInfo is a catalog entry plus detection.
type CatalogInfo struct {
	profile.CatalogEntry
	// Command is the default command (the first binary found).
	Command   string `json:"command"`
	Installed bool   `json:"installed"`
	Path      string `json:"path,omitempty"`
}

type ProfileCatalog struct {
	Entries []CatalogInfo `json:"entries"`
}

// ProfileSaveParams upserts a profiles.yaml entry. Entry holds the desired
// values; fields equal to the inherited ones are not written.
type ProfileSaveParams struct {
	Entry profile.Entry `json:"entry"`
}

type ProfileRef struct {
	ID string `json:"id"`
}

// ProfileReorderParams moves IDs to the front, in order.
type ProfileReorderParams struct {
	IDs []string `json:"ids"`
}

type SpawnParams struct {
	// ProjectID accepts a project id or name.
	ProjectID string `json:"projectId"`
	ProfileID string `json:"profileId"`
	Cols      uint16 `json:"cols,omitempty"`
	Rows      uint16 `json:"rows,omitempty"`
}

// ShutdownParams are daemon.shutdown's. Without Force the daemon refuses
// while any session is still alive (only exited ones may be discarded).
type ShutdownParams struct {
	Force bool `json:"force,omitempty"`
}

type SessionRef struct {
	SessionID string `json:"sessionId"`
}

type ResizeParams struct {
	SessionID string `json:"sessionId"`
	Cols      uint16 `json:"cols"`
	Rows      uint16 `json:"rows"`
}

type NotifyParams struct {
	SessionID string `json:"sessionId"`
	State     string `json:"state"`
	Kind      string `json:"kind,omitempty"`    // permission | question | done | error
	Message   string `json:"message,omitempty"` // shown in notifications
	// Activity describes what the agent is doing ("Leyendo main.go"). With
	// an empty State only the activity is updated.
	Activity string `json:"activity,omitempty"`
	// Turn marks a new turn (prompt submitted): restarts the turn timer.
	Turn bool `json:"turn,omitempty"`
}

// TailParams asks for the last Lines lines of a session's output.
type TailParams struct {
	SessionID string `json:"sessionId"`
	Lines     int    `json:"lines,omitempty"` // default 8, max 200
}

// TailResult is the ANSI-stripped tail of the scrollback, oldest first.
type TailResult struct {
	SessionID string   `json:"sessionId"`
	Lines     []string `json:"lines"`
}

// Progress mirrors a terminal progress report (OSC 9;4). State is
// normal | error | indeterminate | paused; Value is 0-100.
type Progress struct {
	State string `json:"state"`
	Value int    `json:"value"`
}

// Live is what a session is doing right now: its activity line, timers,
// stall flag and progress. It is embedded (flattened) in SessionInfo and in
// session.activity / session.state events.
type Live struct {
	// Activity is a short human description ("Editando main.go", or the
	// last meaningful line of output); ActivitySource is hook | output.
	Activity       string     `json:"activity,omitempty"`
	ActivityAt     *time.Time `json:"activityAt,omitempty"`
	ActivitySource string     `json:"activitySource,omitempty"`
	// TurnStartedAt is when the current (or last) turn started;
	// TurnEndedAt is set once it ended (idle/exited).
	TurnStartedAt *time.Time `json:"turnStartedAt,omitempty"`
	TurnEndedAt   *time.Time `json:"turnEndedAt,omitempty"`
	LastOutputAt  *time.Time `json:"lastOutputAt,omitempty"`
	LastEventAt   *time.Time `json:"lastEventAt,omitempty"`
	// Stalled: a hook-driven turn saw no output nor events for
	// activity.stall_seconds.
	Stalled   bool       `json:"stalled,omitempty"`
	StalledAt *time.Time `json:"stalledAt,omitempty"`
	Progress  *Progress  `json:"progress,omitempty"`
}

// SubscribeParams identifies the subscriber. Client "ui" marks a desktop
// app that shows its own notifications (the daemon then stays quiet).
type SubscribeParams struct {
	Client string `json:"client,omitempty"`
}

// ClientUI is SubscribeParams.Client for desktop UIs.
const ClientUI = "ui"

type AttachParams struct {
	SessionID string `json:"sessionId"`
	// Optional initial size; zero means "leave as is".
	Cols uint16 `json:"cols,omitempty"`
	Rows uint16 `json:"rows,omitempty"`
}

// SessionInfo describes a session on the wire.
type SessionInfo struct {
	ID         string `json:"id"`
	ProjectID  string `json:"projectId"`
	ProjectDir string `json:"projectDir"`
	ProfileID  string `json:"profileId"`
	// ProjectName is the project's display name (for notifications).
	ProjectName string    `json:"projectName,omitempty"`
	Cmd         string    `json:"cmd"`
	Args        []string  `json:"args,omitempty"`
	Cwd         string    `json:"cwd"`
	Pid         int       `json:"pid"`
	CreatedAt   time.Time `json:"createdAt"`
	State       string    `json:"state"`
	ExitCode    *int      `json:"exitCode,omitempty"`
	Cols        uint16    `json:"cols"`
	Rows        uint16    `json:"rows"`
	// Attention: why the session needs the user. AttentionKind
	// (permission | question | done | error) is empty when it doesn't.
	// Unread stays true until session.ack (or the user types).
	AttentionKind    string     `json:"attentionKind,omitempty"`
	AttentionMessage string     `json:"attentionMessage,omitempty"`
	AttentionAt      *time.Time `json:"attentionAt,omitempty"`
	Unread           bool       `json:"unread,omitempty"`
	Live
}

type SessionList struct {
	Sessions []SessionInfo `json:"sessions"`
}

// Event types.
const (
	EventSessionSpawned = "session.spawned"
	EventSessionState   = "session.state"
	EventSessionRemoved = "session.removed"
	// EventSessionAttention: the attention changed without a state change
	// (message filled in, acknowledged).
	EventSessionAttention = "session.attention"
	// EventSessionActivity: activity line, progress, stall flag or output
	// heartbeat changed (coalesced: at most ~2 per second per session).
	EventSessionActivity = "session.activity"
)

// Event is pushed to events.subscribe streams.
type Event struct {
	Type      string    `json:"type"`
	SessionID string    `json:"sessionId"`
	State     string    `json:"state,omitempty"`
	PrevState string    `json:"prevState,omitempty"`
	Source    string    `json:"source,omitempty"`
	ExitCode  *int      `json:"exitCode,omitempty"`
	At        time.Time `json:"at"`
	// Attention: why the session needs the user. AttentionKind
	// (permission | question | done | error) is empty when it doesn't.
	// Unread stays true until session.ack (or the user types).
	AttentionKind    string     `json:"attentionKind,omitempty"`
	AttentionMessage string     `json:"attentionMessage,omitempty"`
	AttentionAt      *time.Time `json:"attentionAt,omitempty"`
	Unread           bool       `json:"unread,omitempty"`
	// Raised: a new attention was raised by this event; UIs notify on it.
	Raised bool `json:"raised,omitempty"`
	// ProjectName and ProfileID label notifications.
	ProjectName string `json:"projectName,omitempty"`
	ProfileID   string `json:"profileId,omitempty"`
	// StallRaised (session.activity): the session just became stalled;
	// UIs may show a low-urgency notification (kind "stall"). It is not an
	// attention: it does not count as unread.
	StallRaised bool `json:"stallRaised,omitempty"`
	// Live is filled in session.activity and session.state events.
	Live
}
