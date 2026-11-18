package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elionaz/agenterm/internal/paths"
)

func TestDropsGrabScreenshotThumbnails(t *testing.T) {
	root := t.TempDir()
	old := dropWait
	dropWait = 200 * time.Millisecond
	t.Cleanup(func() { dropWait = old })

	a := &App{paths: paths.In(filepath.Join(root, "home"))}
	drops := filepath.Join(root, "home", "drops")
	tmp := filepath.Join(root, "TemporaryItems", "NSIRD_screencaptureui_abc123")
	os.MkdirAll(tmp, 0o700)

	plain := filepath.Join(root, "notes.txt")
	os.WriteFile(plain, []byte("x"), 0o600)
	shot := filepath.Join(tmp, "Screenshot 1.png")
	os.WriteFile(shot, []byte("png1"), 0o600)
	lost := filepath.Join(tmp, "Screenshot 2.png") // gone before the grab

	// The drop event: the thumbnail is grabbed, then macOS deletes it.
	dropped := []string{plain, shot, lost}
	a.grabDrops(dropped)
	os.Remove(shot)

	got := a.StageDrops(dropped)
	if got[0] != plain {
		t.Errorf("plain file: %q", got[0])
	}
	want := filepath.Join(drops, "Screenshot 1.png")
	if got[1] != want {
		t.Errorf("thumbnail: %q, want %q", got[1], want)
	} else if b, _ := os.ReadFile(want); string(b) != "png1" {
		t.Errorf("grabbed copy holds %q", b)
	}
	if got[2] != lost {
		t.Errorf("lost thumbnail: %q", got[2])
	}

	// StageDrops before the grab (the frontend wins the race): it waits.
	shot3 := filepath.Join(tmp, "Screenshot 3.png")
	os.WriteFile(shot3, []byte("png3"), 0o600)
	res := make(chan []string)
	go func() { res <- a.StageDrops([]string{shot3}) }()
	time.Sleep(20 * time.Millisecond)
	a.grabDrops([]string{shot3})
	if r := <-res; r[0] != filepath.Join(drops, "Screenshot 3.png") {
		t.Errorf("staged before the grab: %q", r[0])
	}

	// Same name again: a second file, the first is kept.
	os.WriteFile(shot, []byte("png1b"), 0o600)
	a.grabDrops([]string{shot})
	if r := a.StageDrops([]string{shot}); r[0] != filepath.Join(drops, "Screenshot 1 2.png") {
		t.Errorf("second grab: %q", r[0])
	}

	// No grab at all (no drop event): the path is typed as is, after dropWait.
	if r := a.StageDrops([]string{filepath.Join(tmp, "never.png")}); r[0] != filepath.Join(tmp, "never.png") {
		t.Errorf("never grabbed: %q", r[0])
	}

	// Old files are pruned, recent ones kept.
	past := time.Now().Add(-2 * dropsKeep)
	os.Chtimes(want, past, past)
	pruneDrops(drops, time.Now().Add(-dropsKeep))
	if _, err := os.Stat(want); !os.IsNotExist(err) {
		t.Errorf("old file not pruned: %v", err)
	}
	if _, err := os.Stat(filepath.Join(drops, "Screenshot 3.png")); err != nil {
		t.Errorf("recent file pruned: %v", err)
	}
}

func TestIsScreenshotTemp(t *testing.T) {
	for p, want := range map[string]bool{
		"/var/folders/x/T/TemporaryItems/NSIRD_screencaptureui_vsJANm/Screenshot.png": true,
		"/Users/me/Desktop/Screenshot.png":                                            false,
		"/tmp/NSIRD_screencaptureui_x":                                                false,
	} {
		if got := isScreenshotTemp(p); got != want {
			t.Errorf("%s: %v", p, got)
		}
	}
}
