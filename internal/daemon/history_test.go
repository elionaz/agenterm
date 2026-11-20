package daemon_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elionaz/panal/internal/apierr"
	"github.com/elionaz/panal/internal/client"
	"github.com/elionaz/panal/internal/notifycmd"
	"github.com/elionaz/panal/internal/paths"
	"github.com/elionaz/panal/internal/protocol"
)

func TestHistoryArchiveSearchResume(t *testing.T) {
	c, p, _ := startDaemon(t)
	ctx := ctxT(t)
	dir := t.TempDir()
	script := filepath.Join(dir, "agent.sh")
	os.WriteFile(script, []byte("#!/bin/sh\necho \"args: $*\"\nprintf 'compiling the redirect fix\\nAPI token=supersecretvalue123\\n'\nread a\n"), 0o755)
	writeProfiles(t, p.Profiles(), "- id: talk\n  name: Talker\n  cmd: "+script+"\n  setup_hooks: claude\n")
	proj, err := c.ProjectAdd(ctx, dir, "hist")
	if err != nil {
		t.Fatal(err)
	}
	evc := eventStream(t, c, protocol.ClientUI)
	sess, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "talk"})
	if err != nil {
		t.Fatal(err)
	}
	waitTail(t, c, sess.ID, "token=")

	// The agent reports its conversation, with a transcript holding a prompt.
	transcript := filepath.Join(dir, "t.jsonl")
	os.WriteFile(transcript, []byte(`{"type":"user","message":{"role":"user","content":"Fix the OAuth redirect"}}`+"\n"), 0o600)
	t.Setenv(protocol.EnvSession, sess.ID)
	t.Setenv(protocol.EnvSock, p.Socket())
	t.Setenv(paths.EnvHome, "")
	var out bytes.Buffer
	payload := `{"session_id":"11111111-2222-3333-4444-555555555555","transcript_path":"` + transcript + `","cwd":"` + dir + `","hook_event_name":"SessionStart"}`
	if err := notifycmd.Run([]string{"--from-hook", "claude"}, strings.NewReader(payload), &out, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Rename(ctx, sess.ID, "oauth work"); err != nil {
		t.Fatal(err)
	}

	if _, err := c.Kill(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	waitEvent(t, evc, "archived", func(e protocol.Event) bool { return e.Type == protocol.EventHistory && e.SessionID == sess.ID })

	hits, err := c.HistorySearch(ctx, protocol.HistorySearchParams{Query: "oauth redirect"})
	if err != nil || len(hits) != 1 {
		t.Fatalf("search = %+v, %v", hits, err)
	}
	e := hits[0].Entry
	if e.ID != sess.ID || e.Agent != "Talker" || e.Name != "oauth work" || e.ProjectName != "hist" || len(e.Prompts) != 1 ||
		e.Resume == nil || e.Resume.SessionID != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("entry = %+v", e)
	}
	srcs := map[string]bool{}
	for _, sn := range hits[0].Snippets {
		srcs[sn.Source] = true
	}
	if !srcs["prompt"] || !srcs["text"] || !srcs["info"] {
		t.Fatalf("snippets = %+v", hits[0].Snippets)
	}
	d, err := c.HistoryGet(ctx, sess.ID)
	if err != nil || !strings.Contains(d.Text, "compiling the redirect fix") || strings.Contains(d.Text, "supersecretvalue123") ||
		!strings.Contains(d.Text, "token=[redacted]") || !d.CanResume {
		t.Fatalf("detail = %+v, %v", d, err)
	}
	if hits, _ := c.HistorySearch(ctx, protocol.HistorySearchParams{Query: "redirect", ProjectID: "nope"}); hits != nil {
		t.Fatalf("unknown project filter = %+v", hits)
	}

	// Shells are not archived.
	sh, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "shell"})
	if err != nil {
		t.Fatal(err)
	}
	runIn(t, c, sh.ID, "echo shellonly")
	if _, err := c.Kill(ctx, sh.ID); err != nil {
		t.Fatal(err)
	}
	waitState(t, c, sh.ID, "exited")
	if hits, _ := c.HistorySearch(ctx, protocol.HistorySearchParams{}); len(hits) != 1 {
		t.Fatalf("after a shell: %d entries", len(hits))
	}

	// Resume: a new session reopens the conversation in the same folder.
	re, err := c.HistoryResume(ctx, protocol.HistoryResumeParams{ID: sess.ID})
	if err != nil {
		t.Fatal(err)
	}
	if re.ID == sess.ID || re.Cwd != dir || re.Name != "oauth work" {
		t.Fatalf("resumed = %+v", re)
	}
	waitTail(t, c, re.ID, "--resume 11111111-2222-3333-4444-555555555555")

	// Delete; a missing entry is a coded error.
	if err := c.HistoryDelete(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.HistoryGet(ctx, sess.ID); !hasCode(err, apierr.HistoryNotFound) {
		t.Fatalf("get after delete: %v", err)
	}
}

func TestHistoryDisabled(t *testing.T) {
	c, p, _ := startDaemon(t)
	ctx := ctxT(t)
	os.WriteFile(p.Settings(), []byte("history:\n  enabled: false\n"), 0o600)
	writeProfiles(t, p.Profiles(), "- id: talk\n  cmd: /bin/sh\n  args: [\"-c\", \"echo hello; read a\"]\n")
	proj, _ := c.ProjectAdd(ctx, t.TempDir(), "off")
	sess, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "talk"})
	if err != nil {
		t.Fatal(err)
	}
	waitTail(t, c, sess.ID, "hello")
	if _, err := c.Kill(ctx, sess.ID); err != nil {
		t.Fatal(err)
	}
	waitState(t, c, sess.ID, "exited")
	if hits, _ := c.HistorySearch(ctx, protocol.HistorySearchParams{}); len(hits) != 0 {
		t.Fatalf("archived with history off: %+v", hits)
	}
}

// waitTail waits until session id's output contains want.
func waitTail(t *testing.T, c *client.Client, id, want string) {
	t.Helper()
	waitFor(t, "output "+want, func() bool {
		lines, _ := c.Tail(ctxT(t), id, 40)
		return strings.Contains(strings.Join(lines, "\n"), want)
	})
}
