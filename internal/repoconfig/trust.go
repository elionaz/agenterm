package repoconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/elionaz/agenterm/internal/fileformat"
)

// TrustFormat is the newest version of trust.json this build reads.
const TrustFormat = 1

// Trust remembers which setup commands the user allowed, per repository:
// a command runs only while it is exactly the one trusted (any change to
// .agenterm.yaml's worktree.setup asks again).
type Trust struct {
	path string
	mu   sync.Mutex
}

type trustEntry struct {
	Repo string `json:"repo"`
	// Setup is the sha256 of the trusted command (the file keeps no
	// commands, only what they were).
	Setup string `json:"setup"`
}

// OpenTrust uses the file at path (created on the first Allow).
func OpenTrust(path string) *Trust { return &Trust{path: path} }

func digest(cmd string) string {
	sum := sha256.Sum256([]byte(cmd))
	return hex.EncodeToString(sum[:])
}

func (t *Trust) load() ([]trustEntry, error) {
	data, err := os.ReadFile(t.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	list, _, err := fileformat.DecodeList[trustEntry](t.path, data, "trusted", TrustFormat)
	return list, err
}

// Allowed reports whether cmd is the setup command trusted for repo.
func (t *Trust) Allowed(repo, cmd string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	list, err := t.load()
	if err != nil {
		return false
	}
	d := digest(cmd)
	for _, e := range list {
		if e.Repo == repo && e.Setup == d {
			return true
		}
	}
	return false
}

// Allow trusts cmd as repo's setup command (replacing what was trusted
// for it before).
func (t *Trust) Allow(repo, cmd string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	list, err := t.load()
	if err != nil {
		return err
	}
	out := list[:0]
	for _, e := range list {
		if e.Repo != repo {
			out = append(out, e)
		}
	}
	out = append(out, trustEntry{Repo: repo, Setup: digest(cmd)})
	data, err := fileformat.EncodeList("trusted", TrustFormat, out)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(t.path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(t.path), ".trust-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), t.path); err != nil {
		return fmt.Errorf("save %s: %w", t.path, err)
	}
	return nil
}
