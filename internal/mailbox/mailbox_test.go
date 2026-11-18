package mailbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elionaz/agenterm/internal/fileformat"
)

func open(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "messages.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return s, path
}

func TestAddTakeAndReload(t *testing.T) {
	s, path := open(t)
	a, err := s.Add(Message{ProjectID: "p", From: "a", To: "b", Body: "hola"})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != 1 || a.At.IsZero() || !a.Pending() {
		t.Fatalf("stored %+v", a)
	}
	if _, err := s.Add(Message{ProjectID: "p", From: "b", To: "a", Body: "qué tal"}); err != nil {
		t.Fatal(err)
	}
	if n := s.Pending("b"); n != 1 {
		t.Fatalf("pending b = %d", n)
	}
	got, err := s.Take("b")
	if err != nil || len(got) != 1 || got[0].Body != "hola" || got[0].DeliveredAt == nil {
		t.Fatalf("take = %+v, %v", got, err)
	}
	if again, _ := s.Take("b"); len(again) != 0 {
		t.Fatalf("taken twice: %+v", again)
	}

	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if l := r.List(Filter{Session: "b"}); len(l) != 2 || l[0].Pending() || !l[1].Pending() {
		t.Fatalf("reloaded %+v", l)
	}
	if m, _ := r.Add(Message{ProjectID: "p", From: "a", To: "b", Body: "x"}); m.ID != 3 {
		t.Fatalf("ids continue after reload: got %d", m.ID)
	}
}

func TestFilters(t *testing.T) {
	s, _ := open(t)
	for _, m := range []Message{
		{ProjectID: "p", From: "a", To: "b", Body: "1"},
		{ProjectID: "p", From: "b", To: "c", Body: "2"},
		{ProjectID: "q", From: "x", To: "y", Body: "3"},
	} {
		if _, err := s.Add(m); err != nil {
			t.Fatal(err)
		}
	}
	if l := s.List(Filter{ProjectID: "p"}); len(l) != 2 {
		t.Fatalf("project: %d", len(l))
	}
	if l := s.List(Filter{Session: "b"}); len(l) != 2 {
		t.Fatalf("session: %d", len(l))
	}
	if l := s.List(Filter{To: "c", PendingOnly: true}); len(l) != 1 || l[0].Body != "2" {
		t.Fatalf("to: %+v", l)
	}
}

func TestLimits(t *testing.T) {
	s, _ := open(t)
	if _, err := s.Add(Message{ProjectID: "p", From: "a", To: "b"}); err != ErrEmpty {
		t.Fatalf("empty: %v", err)
	}
	long := strings.Repeat("é", MaxBody) // 2 bytes each
	m, err := s.Add(Message{ProjectID: "p", From: "a", To: "b", Body: long})
	if err != nil {
		t.Fatal(err)
	}
	if body := strings.TrimSuffix(m.Body, "…"); len(body) > MaxBody || !strings.HasSuffix(m.Body, "…") {
		t.Fatalf("clipped to %d bytes", len(m.Body))
	}
	if _, err := s.Add(Message{ProjectID: "other", From: "x", To: "y", Body: "keep"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxPerProject+5; i++ {
		if _, err := s.Add(Message{ProjectID: "p", From: "a", To: "b", Body: "m"}); err != nil {
			t.Fatal(err)
		}
	}
	if l := s.List(Filter{ProjectID: "p"}); len(l) != MaxPerProject || l[0].Body != "m" {
		t.Fatalf("kept %d, first %q", len(l), l[0].Body[:1])
	}
	if l := s.List(Filter{ProjectID: "other"}); len(l) != 1 {
		t.Fatalf("other project trimmed: %d", len(l))
	}
}

func TestDropTo(t *testing.T) {
	s, _ := open(t)
	s.Add(Message{ProjectID: "p", From: "a", To: "b", Body: "old"})
	s.Take("b")
	s.Add(Message{ProjectID: "p", From: "a", To: "b", Body: "new"})
	if n, err := s.DropTo("b"); n != 1 || err != nil {
		t.Fatalf("drop = %d, %v", n, err)
	}
	if l := s.List(Filter{}); len(l) != 1 || l[0].Body != "old" {
		t.Fatalf("left %+v", l)
	}
}

func TestNewerFormatRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "messages.json")
	if err := os.WriteFile(path, []byte(`{"version": 99, "messages": []}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); !fileformat.IsTooNew(err) {
		t.Fatalf("want too-new error, got %v", err)
	}
}
