// Package repoconfig reads a repository's own panal configuration,
// .panal.yaml at the root of its main checkout (versioned with the
// project, so a team shares it):
//
//	worktree:
//	  copy: [.env, .env.local, config/local.json]
//	  setup: npm install
//	ports:
//	  base: 3000
//	  env: [PORT]
//
// worktree.copy lists files or folders (relative to the repository root;
// simple globs allowed) copied from the main checkout into every new
// worktree: what git does not track (.env…) and the agent needs.
// worktree.setup is a shell command run in a new worktree before the
// agent starts; since a cloned repository could carry any command, the
// daemon runs it only once the user trusted it (see Trust).
package repoconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/elionaz/panal/internal/fileformat"
)

// Names are the file names looked for, in order. The .agenterm ones are
// from before the rename to panal; new files are always .panal.yaml.
var Names = []string{".panal.yaml", ".panal.yml", ".agenterm.yaml", ".agenterm.yml"}

// Format is the newest version of the file this build reads.
const Format = 1

// Config is a repository's configuration. The zero value means "no file".
type Config struct {
	// Path is the file it was read from ("" when there is none).
	Path     string
	Worktree Worktree
	Ports    Ports
}

// Worktree configures new worktrees.
type Worktree struct {
	Copy  []string
	Setup string
}

// Ports configures the port each session is given.
type Ports struct {
	// Base is the first port; 0 means the project did not ask for ports.
	Base int
	// Env are the variables set to the port (default PORT).
	Env []string
}

type file struct {
	Version  int `yaml:"version"`
	Worktree struct {
		Copy  []string `yaml:"copy"`
		Setup *string  `yaml:"setup"`
	} `yaml:"worktree"`
	Ports struct {
		Base *int     `yaml:"base"`
		Env  []string `yaml:"env"`
	} `yaml:"ports"`
}

// Load reads the configuration of the repository whose main checkout is
// root. A missing file is not an error (zero Config).
func Load(root string) (Config, error) {
	for _, n := range Names {
		p := filepath.Join(root, n)
		data, err := os.ReadFile(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return Config{}, err
		}
		c, err := Parse(p, data)
		c.Path = p
		return c, err
	}
	return Config{}, nil
}

// Parse decodes and validates a file's contents (path names it in errors).
func Parse(path string, data []byte) (Config, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	if _, err := fileformat.YAMLVersion(path, &doc, Format); err != nil {
		return Config{}, err
	}
	var f file
	if err := doc.Decode(&f); err != nil && len(doc.Content) > 0 {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	c := Config{Path: path}
	for _, p := range f.Worktree.Copy {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if err := checkRel(p); err != nil {
			return Config{}, fmt.Errorf("%s: worktree.copy: %w", path, err)
		}
		c.Worktree.Copy = append(c.Worktree.Copy, filepath.Clean(p))
	}
	if f.Worktree.Setup != nil {
		c.Worktree.Setup = strings.TrimSpace(*f.Worktree.Setup)
	}
	if f.Ports.Base != nil {
		b := *f.Ports.Base
		if b < 1024 || b > 65000 {
			return Config{}, fmt.Errorf("%s: ports.base: %d is not between 1024 and 65000", path, b)
		}
		c.Ports.Base = b
		for _, e := range f.Ports.Env {
			e = strings.TrimSpace(e)
			if !validEnvName(e) {
				return Config{}, fmt.Errorf("%s: ports.env: %q is not a variable name", path, e)
			}
			c.Ports.Env = append(c.Ports.Env, e)
		}
		if len(c.Ports.Env) == 0 {
			c.Ports.Env = []string{"PORT"}
		}
	}
	return c, nil
}

// checkRel accepts a path (or glob) that stays inside the repository.
func checkRel(p string) error {
	if filepath.IsAbs(p) || strings.HasPrefix(p, "~") {
		return fmt.Errorf("%q: paths are relative to the repository root", p)
	}
	clean := filepath.Clean(p)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("%q: must name something inside the repository", p)
	}
	if clean == ".git" || strings.HasPrefix(clean, ".git/") {
		return fmt.Errorf("%q: .git is not copied", p)
	}
	if _, err := filepath.Match(clean, ""); err != nil {
		return fmt.Errorf("%q: %w", p, err)
	}
	return nil
}

func validEnvName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_', r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

// ErrExists is returned by Create when the repository already has a file.
var ErrExists = errors.New("the repository already has a .panal.yaml")

// Create writes a first .panal.yaml at root that copies the given
// entries, with the other settings commented out as a starting point. It
// returns the file's path.
func Create(root string, copy []string) (string, error) {
	for _, n := range Names {
		if _, err := os.Stat(filepath.Join(root, n)); err == nil {
			return "", ErrExists
		}
	}
	for _, e := range copy {
		if err := checkRel(e); err != nil {
			return "", err
		}
	}
	var b strings.Builder
	b.WriteString("# panal configuration for this repository (commit it to share it).\n")
	b.WriteString("# https://github.com/elionaz/panal/blob/main/docs/worktrees.md#setting-up-new-worktrees\n")
	b.WriteString("worktree:\n")
	b.WriteString("  # Copied from the main folder into every new worktree (git does not track them).\n")
	b.WriteString("  copy:\n")
	for _, e := range copy {
		fmt.Fprintf(&b, "    - %s\n", yamlScalar(e))
	}
	b.WriteString("  # Run in a new worktree before the agent starts (panal asks you to trust it first).\n")
	b.WriteString("  # setup: npm install\n")
	b.WriteString("# Give each session its own port (PORT=3000, 3001, …):\n")
	b.WriteString("# ports:\n")
	b.WriteString("#   base: 3000\n")
	p := filepath.Join(root, Names[0])
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		return "", err
	}
	return p, nil
}

// yamlScalar quotes s when YAML would read it as something else.
func yamlScalar(s string) string {
	out, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Sprintf("%q", s)
	}
	return strings.TrimSpace(string(out))
}
