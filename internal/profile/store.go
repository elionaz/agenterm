package profile

import (
	"bytes"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/elionaz/agenterm/internal/atomicfile"
)

// Store edits profiles.yaml. The file stays the source of truth: every
// operation re-reads it, and writes are atomic with a .bak of the previous
// version. Rewritten files are canonical: one entry per effective profile,
// in menu order, each with only the fields that differ from what it
// inherits (catalog entries without changes become `- id: x`). Custom
// entries are preserved.
type Store struct {
	Path string
	Look Lookup

	mu sync.Mutex
}

// List returns the effective profiles.
func (s *Store) List() ([]Profile, error) {
	return Load(s.Path, s.Look)
}

var idRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Save upserts the user entry e.ID. e describes the desired profile (as an
// edit form would): fields equal to what the profile inherits are dropped,
// so catalog updates keep flowing. A new entry goes right after the profile
// it extends, or last.
func (s *Store) Save(e Entry) ([]Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !idRe.MatchString(e.ID) {
		return nil, fmt.Errorf("invalid id %q: use letters, digits, '.', '_' or '-'", e.ID)
	}
	es, err := ReadEntries(s.Path)
	if err != nil {
		return nil, err
	}
	cur := Resolve(es, s.Look)
	others := slices.DeleteFunc(slices.Clone(es), func(x Entry) bool { return x.ID == e.ID })
	inCatalog := catalogIndex(e.ID) >= 0
	var base Profile
	switch {
	case inCatalog && e.Extends != "":
		return nil, fmt.Errorf("profile %q is built in and cannot extend another one", e.ID)
	case inCatalog:
		base, _ = Find(Resolve(others, s.Look), e.ID)
	case e.Extends != "":
		if e.Extends == e.ID {
			return nil, fmt.Errorf("profile %q cannot extend itself", e.ID)
		}
		var ok bool
		if base, ok = Find(Resolve(others, s.Look), e.Extends); !ok {
			return nil, fmt.Errorf("profile %q extends unknown profile %q", e.ID, e.Extends)
		}
		base.Name = e.ID // a variant's name defaults to its id, not the parent's
	default:
		if e.Cmd == "" {
			return nil, fmt.Errorf("profile %q: missing cmd", e.ID)
		}
	}
	e = minimize(e, base, inCatalog || e.Extends != "")
	e.Order = nil

	// Order: keep the current position, or insert.
	order := ids(cur)
	if !slices.Contains(order, e.ID) {
		at := len(order)
		if i := slices.Index(order, e.Extends); e.Extends != "" && i >= 0 {
			at = i + 1
		}
		order = slices.Insert(order, at, e.ID)
	}
	next := append(others, e)
	if p, _ := Find(Resolve(next, s.Look), e.ID); p.Error != "" {
		return nil, fmt.Errorf("profile %q: %s", e.ID, p.Error)
	}
	return s.write(next, order)
}

// Delete removes the user entry id: a custom profile disappears, a built-in
// one goes back to its catalog defaults (keeping its position).
func (s *Store) Delete(id string) ([]Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	es, err := ReadEntries(s.Path)
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(es, func(x Entry) bool { return x.ID == id })
	inCatalog := catalogIndex(id) >= 0
	if i < 0 || (inCatalog && es[i].isStub()) {
		if inCatalog {
			return nil, fmt.Errorf("profile %q is built in and has no changes to reset (disable it instead)", id)
		}
		return nil, fmt.Errorf("profile not found: %s", id)
	}
	var users []string
	for _, x := range es {
		if x.Extends == id {
			users = append(users, x.ID)
		}
	}
	if len(users) > 0 && !inCatalog {
		return nil, fmt.Errorf("profile %q is extended by %v: delete those first", id, users)
	}
	order := ids(Resolve(es, s.Look))
	es = slices.Delete(es, i, i+1)
	if !inCatalog {
		order = slices.DeleteFunc(order, func(x string) bool { return x == id })
	}
	return s.write(es, order)
}

// Reorder moves the given ids to the front, in that order; the rest keep
// their relative order after them. The first profile is the ⌘N default.
func (s *Store) Reorder(order []string) ([]Profile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	es, err := ReadEntries(s.Path)
	if err != nil {
		return nil, err
	}
	cur := ids(Resolve(es, s.Look))
	seen := make(map[string]bool, len(order))
	for _, id := range order {
		if !slices.Contains(cur, id) {
			return nil, fmt.Errorf("profile not found: %s", id)
		}
		if seen[id] {
			return nil, fmt.Errorf("profile %q listed twice", id)
		}
		seen[id] = true
	}
	next := slices.Clone(order)
	for _, id := range cur {
		if !seen[id] {
			next = append(next, id)
		}
	}
	return s.write(es, next)
}

// write stores entries in canonical form following order (ids without an
// entry become stubs) and returns the new effective profiles.
func (s *Store) write(es []Entry, order []string) ([]Profile, error) {
	byID := make(map[string]Entry, len(es))
	for _, e := range es {
		e.Order = nil
		byID[e.ID] = e
	}
	out := make([]Entry, 0, len(order))
	for _, id := range order {
		e, ok := byID[id]
		if !ok {
			e = Entry{ID: id}
		}
		out = append(out, e)
		delete(byID, id)
	}
	// Entries Resolve did not list (cannot happen today) are kept anyway.
	for _, e := range es {
		if _, ok := byID[e.ID]; ok {
			out = append(out, byID[e.ID])
		}
	}
	data, err := Marshal(out)
	if err != nil {
		return nil, err
	}
	if err := atomicfile.Write(s.Path, data, 0o600); err != nil {
		return nil, err
	}
	return Resolve(out, s.Look), nil
}

const fileHeader = `# agenterm agent profiles. Each entry overrides the catalog agent with the
# same id (missing fields are inherited) or defines a new one (cmd required,
# or extends: <id>). The list order is the menu order; the first one is ⌘N's.
# "enabled: false" hides it. See README.
`

// Marshal encodes entries in the canonical file format.
func Marshal(es []Entry) ([]byte, error) {
	if len(es) == 0 {
		return []byte(fileHeader + "[]\n"), nil
	}
	var buf bytes.Buffer
	buf.WriteString(fileHeader)
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(es); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// minimize clears the fields of e that equal base (what e inherits).
func minimize(e Entry, base Profile, inherits bool) Entry {
	if !inherits {
		base = Profile{Name: e.ID}
	}
	if e.Name == base.Name || (!inherits && e.Name == e.ID) {
		e.Name = ""
	}
	if e.Cmd == base.Cmd && inherits {
		e.Cmd = ""
	}
	if e.Args != nil && slices.Equal(*e.Args, base.Args) {
		e.Args = nil
	}
	if e.Env != nil && maps.Equal(*e.Env, base.Env) {
		e.Env = nil
	}
	if e.AttentionPatterns != nil && slices.Equal(*e.AttentionPatterns, base.AttentionPatterns) {
		e.AttentionPatterns = nil
	}
	if e.Status == base.Status || (e.Status == "heuristic" && base.Status == "") {
		e.Status = ""
	}
	if e.SetupHooks == base.SetupHooks || (e.SetupHooks == "none" && base.SetupHooks == "") {
		e.SetupHooks = ""
	}
	if e.IdleSeconds == base.IdleSeconds {
		e.IdleSeconds = 0
	}
	if e.Enabled != nil && *e.Enabled {
		e.Enabled = nil
	}
	return e
}

func ids(ps []Profile) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.ID
	}
	return out
}
