// Package client talks to the agenterm daemon. It is used by the CLI and is
// meant to be reused by the desktop UI.
package client

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/elionaz/agenterm/internal/apierr"
	"github.com/elionaz/agenterm/internal/forge"
	"github.com/elionaz/agenterm/internal/gitx"
	"github.com/elionaz/agenterm/internal/mailbox"
	"github.com/elionaz/agenterm/internal/paths"
	"github.com/elionaz/agenterm/internal/profile"
	"github.com/elionaz/agenterm/internal/project"
	"github.com/elionaz/agenterm/internal/protocol"
	"github.com/elionaz/agenterm/internal/settings"
	"github.com/elionaz/agenterm/internal/transport"
	"github.com/elionaz/agenterm/internal/version"
)

// Client is a daemon client. Each control call uses its own short-lived
// connection, so a Client is safe for concurrent use.
type Client struct {
	Sock      string
	Transport transport.Transport
	// AutoStart, if set, is invoked once when the daemon is unreachable,
	// after which the dial is retried.
	AutoStart func(ctx context.Context) error

	startOnce sync.Once
	startErr  error
	nextID    atomic.Uint64
}

// New returns a client for the socket at sock.
func New(sock string) *Client {
	return &Client{Sock: sock, Transport: transport.Default()}
}

// SocketFromEnv resolves the daemon socket; see ResolveSocket.
func SocketFromEnv() (string, error) {
	sock, _, err := ResolveSocket()
	return sock, err
}

// ResolveSocket picks the daemon socket from the environment. An explicit
// AGENTERM_HOME always wins (so `AGENTERM_HOME=/tmp/x agenterm stop` run
// inside a session targets /tmp/x, not the session's daemon);
// $AGENTERM_SOCK, which the daemon injects into its sessions, applies only
// when AGENTERM_HOME is unset; otherwise the default home. fromSession
// reports that $AGENTERM_SOCK was used (callers must not auto-start a
// daemon for it).
func ResolveSocket() (sock string, fromSession bool, err error) {
	if os.Getenv(paths.EnvHome) == "" {
		if s := os.Getenv(protocol.EnvSock); s != "" {
			return s, true, nil
		}
	}
	p, err := paths.Default()
	if err != nil {
		return "", false, err
	}
	return p.Socket(), false, nil
}

// RemoteError is an error reported by the daemon, with its code and data
// for user-facing refusals (apierr.Coded).
type RemoteError struct {
	Msg  string
	Code string
	Data json.RawMessage
}

func (e *RemoteError) Error() string     { return e.Msg }
func (e *RemoteError) ErrorCode() string { return e.Code }
func (e *RemoteError) ErrorData() any    { return e.Data }

// remoteError builds the error for a failed response. A method the daemon
// does not know (code unknown_method, or the plain message of daemons before
// 0.4.0) means it is older than this client: say so and how to fix it.
func remoteError(method string, resp protocol.Response) *RemoteError {
	e := &RemoteError{Msg: resp.Error, Code: resp.ErrorCode, Data: resp.ErrorData}
	if e.Code == apierr.UnknownMethod || (e.Code == "" && strings.HasPrefix(e.Msg, "unknown method ")) {
		e.Code = apierr.UnknownMethod
		e.Data, _ = json.Marshal(map[string]string{"method": method})
		e.Msg = fmt.Sprintf("the agenterm daemon does not support %s: it is older than this agenterm (%s). "+
			"Restart it (\"Restart now\" in the app, or `agenterm stop` and run the command again)", method, version.Version)
	}
	return e
}

func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	conn, err := c.Transport.Dial(ctx, c.Sock)
	if err == nil || c.AutoStart == nil {
		if err != nil {
			return nil, fmt.Errorf("connect to daemon at %s: %w", c.Sock, err)
		}
		return conn, nil
	}
	c.startOnce.Do(func() { c.startErr = c.AutoStart(ctx) })
	if c.startErr != nil {
		return nil, fmt.Errorf("start daemon: %w", c.startErr)
	}
	return c.Transport.Dial(ctx, c.Sock)
}

// open dials, sends a request and reads its response, returning the
// connection positioned right after the response line.
func (c *Client) open(ctx context.Context, method string, params, result any) (net.Conn, *bufio.Reader, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, nil, err
	}
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	fail := func(err error) (net.Conn, *bufio.Reader, error) {
		stop()
		conn.Close()
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		return nil, nil, err
	}
	req := protocol.Request{ID: c.nextID.Add(1), Method: method}
	if params != nil {
		if req.Params, err = json.Marshal(params); err != nil {
			return fail(err)
		}
	}
	b, _ := json.Marshal(req)
	if _, err := conn.Write(append(b, '\n')); err != nil {
		return fail(err)
	}
	br := bufio.NewReaderSize(conn, 64*1024)
	line, err := br.ReadBytes('\n')
	if err != nil {
		return fail(fmt.Errorf("read response: %w", err))
	}
	var resp protocol.Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return fail(fmt.Errorf("decode response: %w", err))
	}
	if resp.Error != "" {
		return fail(remoteError(method, resp))
	}
	if result != nil && len(resp.Result) > 0 {
		if err := json.Unmarshal(resp.Result, result); err != nil {
			return fail(fmt.Errorf("decode result: %w", err))
		}
	}
	if !stop() {
		conn.Close()
		return nil, nil, ctx.Err()
	}
	return conn, br, nil
}

// Call performs one control request and decodes its result into result
// (which may be nil).
func (c *Client) Call(ctx context.Context, method string, params, result any) error {
	conn, _, err := c.open(ctx, method, params, result)
	if err != nil {
		return err
	}
	return conn.Close()
}

// Ping checks the daemon is up.
func (c *Client) Ping(ctx context.Context) error {
	return c.Call(ctx, protocol.MethodDaemonPing, nil, nil)
}

// PingInfo is Ping plus the daemon's build (an empty Version means a daemon
// older than 0.2.0).
func (c *Client) PingInfo(ctx context.Context) (protocol.PingResult, error) {
	var r protocol.PingResult
	err := c.Call(ctx, protocol.MethodDaemonPing, nil, &r)
	return r, err
}

// Shutdown asks the daemon to kill all sessions and exit. Without force the
// daemon refuses (RemoteError) while any session is still alive.
func (c *Client) Shutdown(ctx context.Context, force bool) error {
	return c.Call(ctx, protocol.MethodDaemonShutdown, protocol.ShutdownParams{Force: force}, nil)
}

func (c *Client) ProjectAdd(ctx context.Context, path, name string) (project.Project, error) {
	var p project.Project
	err := c.Call(ctx, protocol.MethodProjectAdd, protocol.ProjectAddParams{Path: path, Name: name}, &p)
	return p, err
}

func (c *Client) ProjectList(ctx context.Context) ([]project.Project, error) {
	var r protocol.ProjectList
	err := c.Call(ctx, protocol.MethodProjectList, nil, &r)
	return r.Projects, err
}

func (c *Client) ProjectRemove(ctx context.Context, id string) error {
	return c.Call(ctx, protocol.MethodProjectRemove, protocol.ProjectRef{ID: id}, nil)
}

// UpdateProject changes a project's name or badge (nil fields stay).
func (c *Client) UpdateProject(ctx context.Context, p protocol.ProjectUpdateParams) (project.Project, error) {
	var r project.Project
	err := c.Call(ctx, protocol.MethodProjectUpdate, p, &r)
	return r, err
}

// ReorderProjects moves ids to the front of the project list, in order.
func (c *Client) ReorderProjects(ctx context.Context, ids []string) ([]project.Project, error) {
	var r protocol.ProjectList
	err := c.Call(ctx, protocol.MethodProjectReorder, protocol.ProjectReorderParams{IDs: ids}, &r)
	return r.Projects, err
}

// ReorderSessions moves ids to the front of projectID's sessions, in order.
func (c *Client) ReorderSessions(ctx context.Context, projectID string, ids []string) ([]protocol.SessionInfo, error) {
	var r protocol.SessionList
	err := c.Call(ctx, protocol.MethodSessionReorder, protocol.SessionReorderParams{ProjectID: projectID, IDs: ids}, &r)
	return r.Sessions, err
}

func (c *Client) Profiles(ctx context.Context) ([]profile.Profile, error) {
	var r protocol.ProfileList
	err := c.Call(ctx, protocol.MethodProfileList, nil, &r)
	return r.Profiles, err
}

// ProfileCatalog lists the built-in agents with detection.
func (c *Client) ProfileCatalog(ctx context.Context) ([]protocol.CatalogInfo, error) {
	var r protocol.ProfileCatalog
	err := c.Call(ctx, protocol.MethodProfileCatalog, nil, &r)
	return r.Entries, err
}

// SaveProfile upserts a profiles.yaml entry and returns the new profiles.
func (c *Client) SaveProfile(ctx context.Context, e profile.Entry) ([]profile.Profile, error) {
	return c.profileCall(ctx, protocol.MethodProfileSave, protocol.ProfileSaveParams{Entry: e})
}

// DeleteProfile removes a custom profile or resets a built-in one.
func (c *Client) DeleteProfile(ctx context.Context, id string) ([]profile.Profile, error) {
	return c.profileCall(ctx, protocol.MethodProfileDelete, protocol.ProfileRef{ID: id})
}

// ReorderProfiles moves ids to the front, in order.
func (c *Client) ReorderProfiles(ctx context.Context, ids []string) ([]profile.Profile, error) {
	return c.profileCall(ctx, protocol.MethodProfileReorder, protocol.ProfileReorderParams{IDs: ids})
}

// RescanProfiles re-reads the search PATH and re-detects installed agents.
func (c *Client) RescanProfiles(ctx context.Context) ([]profile.Profile, error) {
	return c.profileCall(ctx, protocol.MethodProfileRescan, nil)
}

// ProfileInstall installs a catalog agent in a new shell session.
func (c *Client) ProfileInstall(ctx context.Context, p protocol.ProfileInstallParams) (protocol.SessionInfo, error) {
	var r protocol.SessionInfo
	err := c.Call(ctx, protocol.MethodProfileInstall, p, &r)
	return r, err
}

func (c *Client) profileCall(ctx context.Context, method string, params any) ([]profile.Profile, error) {
	var r protocol.ProfileList
	err := c.Call(ctx, method, params, &r)
	return r.Profiles, err
}

func (c *Client) Settings(ctx context.Context) (settings.Settings, error) {
	var s settings.Settings
	err := c.Call(ctx, protocol.MethodSettingsGet, nil, &s)
	return s, err
}

// Instructions lists the instruction topics; with project (id or name)
// Composed is what a session of it receives.
func (c *Client) Instructions(ctx context.Context, project string) (protocol.Instructions, error) {
	var r protocol.Instructions
	err := c.Call(ctx, protocol.MethodInstructionsList, protocol.InstructionsParams{ProjectID: project}, &r)
	return r, err
}

// SaveInstruction saves a topic (new when oldSlug is "").
func (c *Client) SaveInstruction(ctx context.Context, t protocol.InstructionTopic, oldSlug, project string) (protocol.Instructions, error) {
	var r protocol.Instructions
	err := c.Call(ctx, protocol.MethodInstructionsSave, protocol.InstructionsSaveParams{Topic: t, OldSlug: oldSlug, ProjectID: project}, &r)
	return r, err
}

// DeleteInstruction removes a topic.
func (c *Client) DeleteInstruction(ctx context.Context, slug, project string) (protocol.Instructions, error) {
	var r protocol.Instructions
	err := c.Call(ctx, protocol.MethodInstructionsDelete, protocol.InstructionsSlugParams{Slug: slug, ProjectID: project}, &r)
	return r, err
}

// ReorderInstructions puts the topics in slugs' order.
func (c *Client) ReorderInstructions(ctx context.Context, slugs []string, project string) (protocol.Instructions, error) {
	var r protocol.Instructions
	err := c.Call(ctx, protocol.MethodInstructionsReorder, protocol.InstructionsReorderParams{Slugs: slugs, ProjectID: project}, &r)
	return r, err
}

// ShowInstruction is one topic's text.
func (c *Client) ShowInstruction(ctx context.Context, slug string) (protocol.InstructionTopicText, error) {
	var r protocol.InstructionTopicText
	err := c.Call(ctx, protocol.MethodInstructionsShow, protocol.InstructionsSlugParams{Slug: slug}, &r)
	return r, err
}

// BrowserStatus says whether sessions get a browser and what it needs.
func (c *Client) BrowserStatus(ctx context.Context) (protocol.BrowserStatus, error) {
	var r protocol.BrowserStatus
	err := c.Call(ctx, protocol.MethodBrowserStatus, nil, &r)
	return r, err
}

// Browser calls browser.prepare, browser.open or browser.close for a
// session's own Chrome.
func (c *Client) Browser(ctx context.Context, method, sessionID string) (protocol.BrowserInfo, error) {
	var r protocol.BrowserInfo
	err := c.Call(ctx, method, protocol.BrowserParams{SessionID: sessionID}, &r)
	return r, err
}

func (c *Client) SaveSettings(ctx context.Context, s settings.Settings) (settings.Settings, error) {
	var out settings.Settings
	err := c.Call(ctx, protocol.MethodSettingsSave, s, &out)
	return out, err
}

func (c *Client) Spawn(ctx context.Context, p protocol.SpawnParams) (protocol.SessionInfo, error) {
	var info protocol.SessionInfo
	err := c.Call(ctx, protocol.MethodSessionSpawn, p, &info)
	return info, err
}

func (c *Client) Sessions(ctx context.Context) ([]protocol.SessionInfo, error) {
	var r protocol.SessionList
	err := c.Call(ctx, protocol.MethodSessionList, nil, &r)
	return r.Sessions, err
}

func (c *Client) Kill(ctx context.Context, id string) (protocol.SessionInfo, error) {
	var info protocol.SessionInfo
	err := c.Call(ctx, protocol.MethodSessionKill, protocol.SessionRef{SessionID: id}, &info)
	return info, err
}

func (c *Client) Resize(ctx context.Context, id string, cols, rows uint16) error {
	return c.Call(ctx, protocol.MethodSessionResize, protocol.ResizeParams{SessionID: id, Cols: cols, Rows: rows}, nil)
}

func (c *Client) Notify(ctx context.Context, id, state string) (protocol.SessionInfo, error) {
	return c.NotifyEvent(ctx, protocol.NotifyParams{SessionID: id, State: state})
}

// NotifyEvent reports an explicit state with optional attention kind and
// message.
func (c *Client) NotifyEvent(ctx context.Context, p protocol.NotifyParams) (protocol.SessionInfo, error) {
	var info protocol.SessionInfo
	err := c.Call(ctx, protocol.MethodSessionNotify, p, &info)
	return info, err
}

// Tail returns the last lines of a session's output as plain text.
func (c *Client) Tail(ctx context.Context, id string, lines int) ([]string, error) {
	var r protocol.TailResult
	err := c.Call(ctx, protocol.MethodSessionTail, protocol.TailParams{SessionID: id, Lines: lines}, &r)
	return r.Lines, err
}

// Resume respawns suspended sessions (one, or all with all=true).
func (c *Client) Resume(ctx context.Context, id string, all bool) (protocol.SessionBatch, error) {
	var r protocol.SessionBatch
	err := c.Call(ctx, protocol.MethodSessionResume, protocol.SuspendedRef{SessionID: id, All: all}, &r)
	return r, err
}

// Discard forgets suspended sessions (one, or all with all=true).
func (c *Client) Discard(ctx context.Context, id string, all bool) (protocol.SessionBatch, error) {
	var r protocol.SessionBatch
	err := c.Call(ctx, protocol.MethodSessionDiscard, protocol.SuspendedRef{SessionID: id, All: all}, &r)
	return r, err
}

// Ack marks a session's attention as seen.
func (c *Client) Ack(ctx context.Context, id string) (protocol.SessionInfo, error) {
	var info protocol.SessionInfo
	err := c.Call(ctx, protocol.MethodSessionAck, protocol.SessionRef{SessionID: id}, &info)
	return info, err
}

// Rename sets a session's display name (empty: back to the profile's).
func (c *Client) Rename(ctx context.Context, id, name string) (protocol.SessionInfo, error) {
	var info protocol.SessionInfo
	err := c.Call(ctx, protocol.MethodSessionRename, protocol.RenameParams{SessionID: id, Name: name}, &info)
	return info, err
}

// Stream is an attached session. Recv must be called from a single
// goroutine; Input/Resize may be called concurrently with Recv.
type Stream struct {
	Info protocol.SessionInfo
	conn net.Conn
	br   *bufio.Reader
	wmu  sync.Mutex
}

// Attach opens a framed stream to a session. cols/rows of 0 keep the
// current size. The context only bounds the handshake; use Close to detach.
func (c *Client) Attach(ctx context.Context, id string, cols, rows uint16) (*Stream, error) {
	st := &Stream{}
	conn, br, err := c.open(ctx, protocol.MethodSessionAttach, protocol.AttachParams{SessionID: id, Cols: cols, Rows: rows}, &st.Info)
	if err != nil {
		return nil, err
	}
	st.conn, st.br = conn, br
	return st, nil
}

// Recv returns the next frame from the daemon.
func (s *Stream) Recv() (protocol.Frame, error) { return protocol.ReadFrame(s.br) }

// Input sends keystrokes / bytes to the PTY.
func (s *Stream) Input(p []byte) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	return protocol.WriteChunked(s.conn, protocol.FrameInput, p)
}

// Resize changes the PTY size.
func (s *Stream) Resize(cols, rows uint16) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	return protocol.WriteFrame(s.conn, protocol.FrameResize, protocol.ResizePayload(cols, rows))
}

// Close detaches; the session keeps running.
func (s *Stream) Close() error { return s.conn.Close() }

// Events is an events.subscribe stream.
type Events struct {
	conn net.Conn
	br   *bufio.Reader
}

// Subscribe opens an event stream.
func (c *Client) Subscribe(ctx context.Context) (*Events, error) {
	return c.SubscribeAs(ctx, "")
}

// SubscribeAs opens an event stream identifying the client kind
// (protocol.ClientUI for desktop apps that show their own notifications).
func (c *Client) SubscribeAs(ctx context.Context, client string) (*Events, error) {
	var params any
	if client != "" {
		params = protocol.SubscribeParams{Client: client}
	}
	conn, br, err := c.open(ctx, protocol.MethodEventsSubscribe, params, nil)
	if err != nil {
		return nil, err
	}
	return &Events{conn: conn, br: br}, nil
}

// Next blocks for the next event. io.EOF-like errors mean the daemon closed
// the stream (shutdown or slow consumer): resubscribe and resync.
func (e *Events) Next() (protocol.Event, error) {
	line, err := e.br.ReadBytes('\n')
	if err != nil {
		return protocol.Event{}, err
	}
	var ev protocol.Event
	if err := json.Unmarshal(line, &ev); err != nil {
		return protocol.Event{}, err
	}
	return ev, nil
}

// Close ends the subscription.
func (e *Events) Close() error { return e.conn.Close() }

// IsRemote reports whether err came from the daemon (as opposed to transport).
func IsRemote(err error) bool {
	var re *RemoteError
	return errors.As(err, &re)
}

// ProjectGit reports whether a project can use worktrees, its branches and
// the default base.
func (c *Client) ProjectGit(ctx context.Context, projectID string) (protocol.ProjectGit, error) {
	var r protocol.ProjectGit
	err := c.Call(ctx, protocol.MethodProjectGit, protocol.ProjectGitParams{ProjectID: projectID}, &r)
	return r, err
}

// SessionDiff returns a session's changes (worktree: against its base;
// same folder: against HEAD). stat skips the patch text.
func (c *Client) SessionDiff(ctx context.Context, id string, stat bool) (protocol.DiffResult, error) {
	var r protocol.DiffResult
	err := c.Call(ctx, protocol.MethodSessionDiff, protocol.DiffParams{SessionID: id, Stat: stat}, &r)
	return r, err
}

// WorktreeDiff returns a worktree's changes against its base (also for
// orphaned worktrees without a session).
func (c *Client) WorktreeDiff(ctx context.Context, id string, stat bool) (protocol.DiffResult, error) {
	var r protocol.DiffResult
	err := c.Call(ctx, protocol.MethodWorktreeDiff, protocol.DiffParams{WorktreeID: id, Stat: stat}, &r)
	return r, err
}

// Worktrees lists a project's agenterm worktrees (live and orphaned).
func (c *Client) Worktrees(ctx context.Context, projectID string) ([]protocol.WorktreeInfo, error) {
	var r protocol.WorktreeList
	err := c.Call(ctx, protocol.MethodWorktreeList, protocol.WorktreeListParams{ProjectID: projectID}, &r)
	return r.Worktrees, err
}

// WorktreeRemove removes a worktree (see protocol.WorktreeRemoveParams).
func (c *Client) WorktreeRemove(ctx context.Context, p protocol.WorktreeRemoveParams) (protocol.WorktreeRemoveResult, error) {
	var r protocol.WorktreeRemoveResult
	err := c.Call(ctx, protocol.MethodWorktreeRemove, p, &r)
	return r, err
}

// WorktreeCopy copies files from the main folder into a worktree (and,
// with Save, writes them into a new .agenterm.yaml).
func (c *Client) WorktreeCopy(ctx context.Context, p protocol.WorktreeCopyParams) (protocol.WorktreeCopyResult, error) {
	var r protocol.WorktreeCopyResult
	err := c.Call(ctx, protocol.MethodWorktreeCopy, p, &r)
	return r, err
}

// WorktreeMerge merges a worktree's branch into its base in the main tree.
func (c *Client) WorktreeMerge(ctx context.Context, id string) (gitx.MergeResult, error) {
	var r gitx.MergeResult
	err := c.Call(ctx, protocol.MethodWorktreeMerge, protocol.WorktreeRef{ID: id}, &r)
	return r, err
}

// WorktreePR pushes a worktree's branch and returns the pull request page
// (dryRun: only the remote and URL that would be used).
func (c *Client) WorktreePR(ctx context.Context, id string, dryRun bool) (gitx.PRResult, error) {
	var r gitx.PRResult
	err := c.Call(ctx, protocol.MethodWorktreePR, protocol.WorktreePRParams{ID: id, DryRun: dryRun}, &r)
	return r, err
}

// ForgeStatus reports the pull request of a session's or worktree's branch.
func (c *Client) ForgeStatus(ctx context.Context, ref protocol.ForgeRef) (protocol.ForgeStatus, error) {
	var r protocol.ForgeStatus
	err := c.Call(ctx, protocol.MethodForgeStatus, ref, &r)
	return r, err
}

// ForgeComments lists the comments of a pull request.
// ForgeList lists the pull requests of a session's branches (forge.list).
func (c *Client) ForgeList(ctx context.Context, p protocol.ForgeListParams) (protocol.ForgeList, error) {
	var r protocol.ForgeList
	err := c.Call(ctx, protocol.MethodForgeList, p, &r)
	return r, err
}

func (c *Client) ForgeComments(ctx context.Context, p protocol.ForgeCommentsParams) ([]forge.Comment, error) {
	var r []forge.Comment
	err := c.Call(ctx, protocol.MethodForgeComments, p, &r)
	return r, err
}

// ForgeMerge merges a pull request on the host.
func (c *Client) ForgeMerge(ctx context.Context, p protocol.ForgeMergeParams) (protocol.ForgeStatus, error) {
	var r protocol.ForgeStatus
	err := c.Call(ctx, protocol.MethodForgeMerge, p, &r)
	return r, err
}

// ForgeCreate pushes the branch and opens its pull request.
func (c *Client) ForgeCreate(ctx context.Context, p protocol.ForgeCreateParams) (protocol.ForgeStatus, error) {
	var r protocol.ForgeStatus
	err := c.Call(ctx, protocol.MethodForgeCreate, p, &r)
	return r, err
}

// WorktreeOpen opens a worktree (or a session's folder) in the editor or
// Finder.
func (c *Client) WorktreeOpen(ctx context.Context, p protocol.WorktreeOpenParams) error {
	return c.Call(ctx, protocol.MethodWorktreeOpen, p, nil)
}

// MsgPeers lists the sessions of the caller's project (or, outside a
// session, of sessionID's).
func (c *Client) MsgPeers(ctx context.Context, sessionID string) (protocol.MsgPeers, error) {
	var r protocol.MsgPeers
	err := c.Call(ctx, protocol.MethodMsgPeers, protocol.MsgPeersParams{SessionID: sessionID}, &r)
	return r, err
}

// MsgSend sends a message (see protocol.MsgSendParams).
func (c *Client) MsgSend(ctx context.Context, p protocol.MsgSendParams) ([]mailbox.Message, error) {
	var r protocol.MsgList
	err := c.Call(ctx, protocol.MethodMsgSend, p, &r)
	return r.Messages, err
}

// MsgList lists messages (a session sees only its own).
func (c *Client) MsgList(ctx context.Context, p protocol.MsgListParams) ([]mailbox.Message, error) {
	var r protocol.MsgList
	err := c.Call(ctx, protocol.MethodMsgList, p, &r)
	return r.Messages, err
}

// MsgTake returns the calling session's pending messages and marks them
// delivered.
func (c *Client) MsgTake(ctx context.Context, intro bool) (protocol.MsgTakeResult, error) {
	var r protocol.MsgTakeResult
	err := c.Call(ctx, protocol.MethodMsgTake, protocol.MsgTakeParams{Intro: intro}, &r)
	return r, err
}

// MsgTakePeek is MsgTake without marking them delivered (see MsgAck).
func (c *Client) MsgTakePeek(ctx context.Context, intro bool) (protocol.MsgTakeResult, error) {
	var r protocol.MsgTakeResult
	err := c.Call(ctx, protocol.MethodMsgTake, protocol.MsgTakeParams{Intro: intro, Peek: true}, &r)
	return r, err
}

// MsgAck marks the calling session's messages ids delivered.
func (c *Client) MsgAck(ctx context.Context, ids []int) error {
	return c.Call(ctx, protocol.MethodMsgAck, protocol.MsgAckParams{IDs: ids}, nil)
}

// MsgDeliver nudges sessionID about its pending messages (the user only).
func (c *Client) MsgDeliver(ctx context.Context, sessionID string) (int, error) {
	var r struct {
		Pending int `json:"pending"`
	}
	err := c.Call(ctx, protocol.MethodMsgDeliver, protocol.MsgDeliverParams{SessionID: sessionID}, &r)
	return r.Pending, err
}
