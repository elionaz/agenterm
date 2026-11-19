// Package instructions keeps the user's standing instructions for the
// agents agenterm launches, as topics (security, architecture, git…): one
// Markdown file per topic in <config>/instructions/<slug>.md, with a YAML
// front matter that says how it loads and which projects it applies to.
// They stay out of the repository (a team's shared instructions belong in
// its AGENTS.md / CLAUDE.md) and out of the agents' own configuration:
// agenterm hands them over when a session starts (Claude Code's
// SessionStart hook, codex's developer_instructions).
//
// A topic loads "always" (its whole text at start) or "on_demand" (the
// session only gets its title and description, and reads the text with
// `agenterm instructions show <slug>` when it applies, like a skill).
package instructions

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
	"gopkg.in/yaml.v3"

	"github.com/elionaz/agenterm/internal/atomicfile"
)

// Limits: each topic's text, and what loads "always" in a session (it
// goes into every session's context).
const (
	MaxBytes = 16 << 10
	MaxTotal = 32 << 10
)

// Load modes.
const (
	LoadAlways   = "always"
	LoadOnDemand = "on_demand"
)

// Topic is one instructions file.
type Topic struct {
	Slug        string `json:"slug" yaml:"-"`
	Title       string `json:"title" yaml:"title"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Load        string `json:"load" yaml:"load"`
	// Projects are the project ids it applies to; nil or empty: all.
	Projects []string `json:"projects,omitempty" yaml:"-"`
	Enabled  bool     `json:"enabled" yaml:"enabled"`
	Order    int      `json:"order" yaml:"order"`
	Body     string   `json:"body" yaml:"-"`
}

// AllProjects reports whether t applies to every project.
func (t Topic) AllProjects() bool { return len(t.Projects) == 0 }

// AppliesTo reports whether t is on for project id.
func (t Topic) AppliesTo(id string) bool {
	return t.Enabled && strings.TrimSpace(t.Body) != "" && (t.AllProjects() || slices.Contains(t.Projects, id))
}

// header is the front matter as written (projects: all | [ids]).
type header struct {
	Title       string    `yaml:"title"`
	Description string    `yaml:"description,omitempty"`
	Load        string    `yaml:"load"`
	Projects    yaml.Node `yaml:"projects"`
	Enabled     *bool     `yaml:"enabled,omitempty"`
	Order       int       `yaml:"order"`
}

// Store reads and writes the topic files under a config directory.
type Store struct{ dir string }

// New returns the Store of config directory dir.
func New(dir string) Store { return Store{dir: filepath.Join(dir, "instructions")} }

// Dir is the folder of the topic files.
func (s Store) Dir() string { return s.dir }

var validSlug = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidSlug reports whether slug can name a topic file.
func ValidSlug(slug string) bool { return len(slug) <= 64 && validSlug.MatchString(slug) }

// Path is the file of topic slug ("" for an invalid slug).
func (s Store) Path(slug string) string {
	if !ValidSlug(slug) {
		return ""
	}
	return filepath.Join(s.dir, slug+".md")
}

// Slugify turns a title into a slug ("Data management" → data-management,
// "Seguridad / Auth" → seguridad-auth); "topic" when nothing is left.
func Slugify(title string) string {
	var b strings.Builder
	dash := false
	for _, r := range norm.NFD.String(strings.ToLower(title)) {
		switch {
		case unicode.Is(unicode.Mn, r): // accents
		case r < utf8.RuneSelf && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
		default:
			dash = true
		}
	}
	out := b.String()
	if len(out) > 48 {
		out = strings.TrimRight(out[:48], "-")
	}
	if out == "" {
		return "topic"
	}
	return out
}

// List is every topic, by Order then title. Files that don't parse are
// left out (errs says which).
func (s Store) List() (topics []Topic, errs []error) {
	ents, err := os.ReadDir(s.dir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
		return nil, errs
	}
	for _, e := range ents {
		slug, ok := strings.CutSuffix(e.Name(), ".md")
		if !ok || e.IsDir() || !ValidSlug(slug) {
			continue
		}
		t, err := s.Get(slug)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		topics = append(topics, t)
	}
	sortTopics(topics)
	return topics, errs
}

func sortTopics(ts []Topic) {
	slices.SortStableFunc(ts, func(a, b Topic) int {
		if a.Order != b.Order {
			return a.Order - b.Order
		}
		return strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
	})
}

// ErrNotFound: no topic with that slug.
var ErrNotFound = errors.New("instructions topic not found")

// Get reads topic slug.
func (s Store) Get(slug string) (Topic, error) {
	p := s.Path(slug)
	if p == "" {
		return Topic{}, fmt.Errorf("invalid topic %q", slug)
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return Topic{}, fmt.Errorf("%w: %s", ErrNotFound, slug)
	}
	if err != nil {
		return Topic{}, err
	}
	t, err := Parse(b)
	if err != nil {
		return Topic{}, fmt.Errorf("%s: %w", p, err)
	}
	t.Slug = slug
	if t.Title == "" {
		t.Title = slug
	}
	return t, nil
}

// Parse reads a topic file: an optional front matter between "---" lines,
// then the text. Without front matter the whole file is the text (always,
// all projects).
func Parse(b []byte) (Topic, error) {
	t := Topic{Load: LoadAlways, Enabled: true}
	text := strings.ReplaceAll(strings.ToValidUTF8(string(b), ""), "\r\n", "\n")
	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		t.Body = strings.TrimRight(text, " \t\n")
		return t, nil
	}
	if strings.HasPrefix(rest, "---") {
		rest = "\n" + rest // empty front matter
	}
	fm, body, ok := strings.Cut(rest, "\n---")
	if !ok {
		return t, errors.New("front matter has no closing ---")
	}
	body = strings.TrimLeft(body, "\n")
	var h header
	if err := yaml.Unmarshal([]byte(fm), &h); err != nil {
		return t, fmt.Errorf("front matter: %w", err)
	}
	t.Title, t.Description, t.Order = strings.TrimSpace(h.Title), strings.TrimSpace(h.Description), h.Order
	t.Body = strings.TrimRight(body, " \t\n")
	if h.Load == LoadOnDemand {
		t.Load = LoadOnDemand
	}
	if h.Enabled != nil {
		t.Enabled = *h.Enabled
	}
	switch h.Projects.Kind {
	case yaml.SequenceNode:
		var ids []string
		if err := h.Projects.Decode(&ids); err != nil {
			return t, fmt.Errorf("projects: %w", err)
		}
		for _, id := range ids {
			if id = strings.TrimSpace(id); id != "" && !slices.Contains(t.Projects, id) {
				t.Projects = append(t.Projects, id)
			}
		}
	case yaml.ScalarNode, 0: // "all" or missing
	default:
		return t, errors.New("projects: want all or a list of project ids")
	}
	return t, nil
}

// Format writes t as a topic file.
func Format(t Topic) []byte {
	h := map[string]any{"title": t.Title, "load": t.Load, "enabled": t.Enabled, "order": t.Order}
	if t.Description != "" {
		h["description"] = t.Description
	}
	if t.AllProjects() {
		h["projects"] = "all"
	} else {
		h["projects"] = t.Projects
	}
	// Fixed key order, so files diff well when edited by hand.
	var b bytes.Buffer
	b.WriteString("---\n")
	for _, k := range []string{"title", "description", "load", "projects", "enabled", "order"} {
		v, ok := h[k]
		if !ok {
			continue
		}
		y, _ := yaml.Marshal(map[string]any{k: v})
		if k == "projects" && !t.AllProjects() {
			y, _ = yaml.Marshal(map[string]any{k: flowSeq(t.Projects)})
		}
		b.Write(y)
	}
	b.WriteString("---\n\n")
	body := strings.TrimSpace(t.Body)
	if body != "" {
		b.WriteString(body)
		b.WriteByte('\n')
	}
	return b.Bytes()
}

func flowSeq(ids []string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
	for _, id := range ids {
		n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: id})
	}
	return n
}

// Check validates t before it is saved.
func Check(t Topic) error {
	switch {
	case strings.TrimSpace(t.Title) == "":
		return errors.New("a topic needs a title")
	case len(t.Title) > 80 || strings.ContainsAny(t.Title, "\n\r"):
		return errors.New("the title must be one line of at most 80 bytes")
	case len(t.Description) > 200 || strings.ContainsAny(t.Description, "\n\r"):
		return errors.New("the description must be one line of at most 200 bytes")
	case t.Load != LoadAlways && t.Load != LoadOnDemand:
		return fmt.Errorf("load must be %s or %s", LoadAlways, LoadOnDemand)
	case len(t.Body) > MaxBytes:
		return fmt.Errorf("the text is %d bytes, the limit is %d", len(t.Body), MaxBytes)
	case !utf8.ValidString(t.Title + t.Description + t.Body):
		return errors.New("instructions are not valid UTF-8")
	}
	return nil
}

// Save writes t. A new topic (oldSlug "") gets a slug from its title,
// made unique; a renamed one keeps its slug. It returns t as saved.
func (s Store) Save(t Topic, oldSlug string) (Topic, error) {
	t.Title, t.Description = strings.TrimSpace(t.Title), strings.TrimSpace(t.Description)
	if t.Load == "" {
		t.Load = LoadAlways
	}
	if err := Check(t); err != nil {
		return t, err
	}
	if oldSlug != "" {
		if _, err := s.Get(oldSlug); err != nil {
			return t, err
		}
		t.Slug = oldSlug
	} else {
		base := Slugify(t.Title)
		t.Slug = base
		for i := 2; ; i++ {
			if _, err := os.Stat(s.Path(t.Slug)); errors.Is(err, os.ErrNotExist) {
				break
			}
			t.Slug = fmt.Sprintf("%s-%d", base, i)
		}
		if t.Order == 0 {
			all, _ := s.List()
			for _, x := range all {
				t.Order = max(t.Order, x.Order)
			}
			t.Order += 10
		}
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return t, err
	}
	if err := atomicfile.Write(s.Path(t.Slug), Format(t), 0o600); err != nil {
		return t, err
	}
	t.Body = strings.TrimSpace(t.Body)
	return t, nil
}

// Delete removes topic slug.
func (s Store) Delete(slug string) error {
	p := s.Path(slug)
	if p == "" {
		return fmt.Errorf("invalid topic %q", slug)
	}
	if err := os.Remove(p); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %s", ErrNotFound, slug)
	} else if err != nil {
		return err
	}
	return nil
}

// Reorder puts the topics in slugs' order (others after them, as they
// were) and rewrites the order of those that changed.
func (s Store) Reorder(slugs []string) error {
	all, _ := s.List()
	rank := func(t Topic) int {
		if i := slices.Index(slugs, t.Slug); i >= 0 {
			return i
		}
		return len(slugs)
	}
	slices.SortStableFunc(all, func(a, b Topic) int { return rank(a) - rank(b) })
	for i, t := range all {
		if want := (i + 1) * 10; t.Order != want {
			t.Order = want
			if err := atomicfile.Write(s.Path(t.Slug), Format(t), 0o600); err != nil {
				return err
			}
		}
	}
	return nil
}

// For composes what a session of project (id, name) receives at start;
// "" when no topic applies.
func (s Store) For(id, name string) string {
	ts, _ := s.List()
	return Compose(ts, id, name)
}

// Compose joins the topics that apply to project id: the "always" ones in
// full, then a list of the "on_demand" ones with how to read them. The
// always texts stop at MaxTotal (a note says what was cut).
func Compose(ts []Topic, id, projectName string) string {
	var always, demand []Topic
	for _, t := range ts {
		if !t.AppliesTo(id) {
			continue
		}
		if t.Load == LoadOnDemand {
			demand = append(demand, t)
		} else {
			always = append(always, t)
		}
	}
	if len(always) == 0 && len(demand) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Standing instructions from your user (set in agenterm; follow them in this session")
	if projectName != "" {
		fmt.Fprintf(&b, " of project %q", projectName)
	}
	b.WriteString("):")
	total := 0
	var cut []string
	for _, t := range always {
		body := strings.TrimSpace(t.Body)
		if total+len(body) > MaxTotal {
			cut = append(cut, t.Slug)
			continue
		}
		total += len(body)
		fmt.Fprintf(&b, "\n\n## %s\n\n%s", t.Title, body)
	}
	if len(demand) > 0 || len(cut) > 0 {
		b.WriteString("\n\n## More instructions, read them when they apply\n\n" +
			"Before work that touches one of these topics, run `agenterm instructions show <topic>` and follow it:\n")
		for _, t := range demand {
			fmt.Fprintf(&b, "\n- %s", t.Slug)
			if d := cmpOr(t.Description, t.Title); d != t.Slug {
				fmt.Fprintf(&b, ": %s", d)
			}
		}
		for _, slug := range cut {
			fmt.Fprintf(&b, "\n- %s (too long to include here)", slug)
		}
	}
	return b.String()
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
