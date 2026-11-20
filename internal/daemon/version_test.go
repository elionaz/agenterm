package daemon_test

import (
	"testing"

	"github.com/elionaz/panal/internal/protocol"
	"github.com/elionaz/panal/internal/version"
)

// daemon.ping reports the daemon's build, which is how the app notices a
// daemon left running by an older version after an update.
func TestPingReportsVersion(t *testing.T) {
	oldV, oldC := version.Version, version.Commit
	// Registered before startDaemon: runs after the daemon stopped.
	t.Cleanup(func() { version.Version, version.Commit = oldV, oldC })
	version.Version, version.Commit = "9.8.7-test", "abc1234"

	c, _, _ := startDaemon(t)
	got, err := c.PingInfo(ctxT(t))
	if err != nil {
		t.Fatal(err)
	}
	want := protocol.PingResult{OK: true, Version: "9.8.7-test", Commit: "abc1234", Protocol: protocol.Version, Exe: "/opt/panal test/panal"}
	if got != want {
		t.Fatalf("daemon.ping = %+v, want %+v", got, want)
	}
	// The plain Ping keeps working (old clients ignore the new fields).
	if err := c.Ping(ctxT(t)); err != nil {
		t.Fatal(err)
	}
}
