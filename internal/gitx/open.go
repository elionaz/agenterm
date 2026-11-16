package gitx

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Open targets.
const (
	OpenEditor = "editor"
	OpenFinder = "finder"
)

// OpenArgv is the command that opens dir for target. editor is the
// git.editor setting: a command on PATH ("code", "cursor -n", "zed"), an
// absolute path, or a macOS application name ("Visual Studio Code",
// "Cursor.app") opened with `open -a`. Empty means `code` when it is on
// PATH, else Finder. look resolves commands (exec.LookPath-like).
func OpenArgv(target, editor, dir string, look func(string) (string, error)) ([]string, error) {
	switch target {
	case OpenFinder:
		return []string{"open", dir}, nil
	case OpenEditor, "":
	default:
		return nil, fmt.Errorf("unknown target %q (editor or finder)", target)
	}
	editor = strings.TrimSpace(editor)
	if editor == "" {
		if p, err := look("code"); err == nil {
			return []string{p, dir}, nil
		}
		return []string{"open", dir}, nil
	}
	if strings.HasSuffix(editor, ".app") {
		return []string{"open", "-a", editor, dir}, nil
	}
	f := strings.Fields(editor)
	if p, err := look(f[0]); err == nil {
		return append(append([]string{p}, f[1:]...), dir), nil
	}
	if strings.HasPrefix(editor, "/") {
		return nil, fmt.Errorf("editor not found: %q", editor)
	}
	// Not a command: an application name ("Visual Studio Code").
	return []string{"open", "-a", editor, dir}, nil
}

// Launch starts argv detached from the caller and waits briefly for an
// early failure (an unknown `open -a` application exits non-zero at once).
func Launch(ctx context.Context, argv []string, env []string) error {
	if len(argv) == 0 {
		return errors.New("empty command")
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = env
	var out strings.Builder
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			msg := strings.TrimSpace(out.String())
			if msg == "" {
				msg = err.Error()
			}
			return fmt.Errorf("%s: %s", strings.Join(argv, " "), msg)
		}
		return nil
	case <-time.After(1500 * time.Millisecond):
		return nil // still running (an editor's CLI that stays up): fine
	case <-ctx.Done():
		return nil
	}
}
