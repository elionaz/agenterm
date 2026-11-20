package daemon_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elionaz/panal/internal/protocol"
)

// fakeOpencode is an "opencode" that answers --version with version and
// otherwise writes its OPENCODE_CONFIG_CONTENT to $OUT.
func fakeOpencode(t *testing.T, version string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "opencode")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = --version ]; then echo 'opencode v" + version + "'; exit 0; fi\n" +
		"printf '%s' \"$OPENCODE_CONFIG_CONTENT\" > \"$OUT.tmp\" && mv \"$OUT.tmp\" \"$OUT\"\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOpencodeIntegration(t *testing.T) {
	t.Setenv("OPENCODE_CONFIG_CONTENT", "")
	c, p, stop := startDaemon(t)
	defer stop()
	out := t.TempDir()
	v2, v1 := fakeOpencode(t, "2.1.4"), fakeOpencode(t, "1.19.6")
	writeProfiles(t, p.Profiles(), "- id: oc2\n  cmd: "+v2+"\n  setup_hooks: opencode\n  env: {OUT: "+filepath.Join(out, "oc2")+"}\n"+
		"- id: oc1\n  cmd: "+v1+"\n  setup_hooks: opencode\n  env: {OUT: "+filepath.Join(out, "oc1")+"}\n")
	ctx := context.Background()
	proj, err := c.ProjectAdd(ctx, t.TempDir(), "web")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.SaveInstruction(ctx, protocol.InstructionTopic{Title: "General", Load: "always", Enabled: true, Body: "Use pnpm."}, "", ""); err != nil {
		t.Fatal(err)
	}

	info, err := c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "oc2"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(info.Args, "--standalone") {
		t.Fatalf("args %q", info.Args)
	}
	var cfg struct {
		Plugins []struct {
			Package string `json:"package"`
		} `json:"plugins"`
		Instructions []string `json:"instructions"`
	}
	file := filepath.Join(out, "oc2")
	waitFor(t, "opencode's config", func() bool { _, err := os.Stat(file); return err == nil })
	data, _ := os.ReadFile(file)
	if err := json.Unmarshal(data, &cfg); err != nil || len(cfg.Plugins) != 1 || len(cfg.Instructions) != 1 {
		t.Fatalf("config %s (%v)", data, err)
	}
	if js, err := os.ReadFile(filepath.Join(cfg.Plugins[0].Package, "index.js")); err != nil || !strings.Contains(string(js), "--from-hook") {
		t.Fatalf("plugin: %v", err)
	}
	if txt, err := os.ReadFile(cfg.Instructions[0]); err != nil || !strings.Contains(string(txt), "Use pnpm.") {
		t.Fatalf("instructions %q: %s %v", cfg.Instructions[0], txt, err)
	}
	if ents, _ := os.ReadDir(proj.Path); len(ents) != 0 {
		t.Fatalf("project folder written: %v", ents)
	}

	// opencode 1.x: launched as it is, no integration.
	info, err = c.Spawn(ctx, protocol.SpawnParams{ProjectID: proj.ID, ProfileID: "oc1"})
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(info.Args, "--standalone") {
		t.Fatalf("opencode 1 args %q", info.Args)
	}
	file = filepath.Join(out, "oc1")
	waitFor(t, "opencode 1 to run", func() bool { _, err := os.Stat(file); return err == nil })
	if data, _ := os.ReadFile(file); len(data) != 0 {
		t.Fatalf("opencode 1 got a config: %s", data)
	}
}
