package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOnboardingFindsTheCLI(t *testing.T) {
	t.Setenv("PANAL_NOTIFY", "off")
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	a := NewApp()
	if info := a.Onboarding(); info.CLIPath != "" || info.Notifications != "off" {
		t.Fatalf("empty PATH: %+v", info)
	}
	cli := filepath.Join(dir, "panal")
	if err := os.WriteFile(cli, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if info := a.Onboarding(); info.CLIPath != cli {
		t.Fatalf("CLI on PATH: %+v", info)
	}
	if s := a.RequestNotifications(); s != "off" {
		t.Fatalf("request with notifications off: %q", s)
	}
}
