package client

import (
	"path/filepath"
	"testing"

	"github.com/elionaz/panal/internal/paths"
	"github.com/elionaz/panal/internal/protocol"
)

func TestResolveSocketPrecedence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	def := filepath.Join(home, ".panal", "d.sock")
	cases := []struct {
		name            string
		panalHome       string
		sock            string
		want            string
		wantFromSession bool
	}{
		{"default", "", "", def, false},
		{"inside a session", "", "/s/d.sock", "/s/d.sock", true},
		{"explicit home", "/tmp/x", "", "/tmp/x/d.sock", false},
		// The bug this guards: PANAL_HOME=/tmp/x panal stop run inside
		// a session must not reach the session's (real) daemon.
		{"explicit home wins over session socket", "/tmp/x", "/real/d.sock", "/tmp/x/d.sock", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(paths.EnvHome, c.panalHome)
			t.Setenv(protocol.EnvSock, c.sock)
			got, fromSession, err := ResolveSocket()
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want || fromSession != c.wantFromSession {
				t.Fatalf("got %q (fromSession %v), want %q (%v)", got, fromSession, c.want, c.wantFromSession)
			}
			if s, _ := SocketFromEnv(); s != c.want {
				t.Fatalf("SocketFromEnv = %q", s)
			}
		})
	}
}
