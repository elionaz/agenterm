package daemon_test

import (
	"strings"
	"testing"

	"github.com/elionaz/panal/internal/daemon"
	"github.com/elionaz/panal/internal/protocol"
)

func TestProfileInstall(t *testing.T) {
	defer daemon.SetInstallCommand("echo fake-install-ran")()
	c, _, stop := startDaemon(t)
	defer stop()
	ctx := ctxT(t)
	proj, err := c.ProjectAdd(ctx, t.TempDir(), "inst")
	if err != nil {
		t.Fatal(err)
	}
	// Not a catalog agent with an install command: refused.
	for _, id := range []string{"shell", "my-own-agent"} {
		if _, err := c.ProfileInstall(ctx, protocol.ProfileInstallParams{ProfileID: id, ProjectID: proj.ID}); err == nil {
			t.Fatalf("install %s accepted", id)
		}
	}
	// A catalog agent: a shell session runs the command, titled, in view.
	info, err := c.ProfileInstall(ctx, protocol.ProfileInstallParams{ProfileID: "codex", ProjectID: proj.ID})
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "install codex" || info.ProfileID != "shell" {
		t.Fatalf("session = %+v", info)
	}
	st, err := c.Attach(ctx, info.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	out := readUntil(t, st, func(out []byte, _ protocol.Frame) bool {
		return strings.Contains(string(out), "fake-install-ran")
	})
	if !strings.Contains(string(out), "installing Codex") {
		t.Fatalf("no title in:\n%s", out)
	}
	runIn(t, c, info.ID, "true") // the shell stays open afterwards
}
