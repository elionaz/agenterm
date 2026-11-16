package i18n

import (
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestMatch(t *testing.T) {
	cases := []struct {
		tags []string
		want Lang
		ok   bool
	}{
		{[]string{"es-ES", "en"}, ES, true},
		{[]string{"fr-FR", "es-419", "en-US"}, ES, true},
		{[]string{"en_GB.UTF-8"}, EN, true},
		{[]string{"ES"}, ES, true},
		{[]string{"fr", "de"}, "", false},
		{nil, "", false},
	}
	for _, c := range cases {
		if got, ok := Match(c.tags); got != c.want || ok != c.ok {
			t.Errorf("Match(%v) = %q, %v; want %q, %v", c.tags, got, ok, c.want, c.ok)
		}
	}
}

func TestDetect(t *testing.T) {
	apple := func(tags ...string) func() []string { return func() []string { return tags } }
	cases := []struct {
		name string
		d    Detector
		want Lang
	}{
		{"apple wins over LANG", Detector{AppleLanguages: apple("es-ES", "en"), Getenv: env(map[string]string{"LANG": "en_US.UTF-8"})}, ES},
		{"apple unsupported falls to LANG", Detector{AppleLanguages: apple("fr"), Getenv: env(map[string]string{"LANG": "es_ES.UTF-8"})}, ES},
		{"LC_ALL before LANG", Detector{Getenv: env(map[string]string{"LC_ALL": "en_US.UTF-8", "LANG": "es_ES"})}, EN},
		{"C locale skipped", Detector{Getenv: env(map[string]string{"LC_ALL": "C", "LANG": "es_MX"})}, ES},
		{"first set variable wins", Detector{Getenv: env(map[string]string{"LC_MESSAGES": "fr_FR", "LANG": "es_ES"})}, EN},
		{"nothing: English", Detector{}, EN},
	}
	for _, c := range cases {
		if got := c.d.Detect(); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestResolve(t *testing.T) {
	d := Detector{AppleLanguages: func() []string { return []string{"es"} }}
	if d.Resolve("en") != EN || d.Resolve(" ES ") != ES || d.Resolve("auto") != ES || d.Resolve("") != ES || d.Resolve("klingon") != ES {
		t.Fatal("resolve")
	}
	if NormalizeSetting("Es") != "es" || NormalizeSetting("x") != Auto || NormalizeSetting("") != Auto {
		t.Fatal("normalize")
	}
}

func TestMessagesComplete(t *testing.T) {
	for k, m := range messages {
		for _, l := range []Lang{EN, ES} {
			if strings.TrimSpace(m[l]) == "" {
				t.Errorf("%s: missing %s", k, l)
			}
		}
		if strings.Count(m[EN], "%") != strings.Count(m[ES], "%") {
			t.Errorf("%s: verbs differ between languages", k)
		}
	}
	if T(ES, PermissionFor, "Bash") != "Permiso para Bash" || T(EN, PermissionFor, "Bash") != "Permission for Bash" {
		t.Fatal("T")
	}
	if T("fr", NotifyDone) != "Finished" || T(EN, "nope") != "nope" {
		t.Fatal("fallbacks")
	}
}

func TestActivityText(t *testing.T) {
	cases := []struct {
		l            Lang
		kind, target string
		want         string
	}{
		{EN, ActEdit, "main.go", "Editing main.go"},
		{ES, ActRead, "go.mod", "Leyendo go.mod"},
		{EN, ActRead, "", "Reading"},
		{ES, ActThinking, "", "Pensando…"},
		{EN, ActTodo, "ignored", "Updating tasks"},
		{ES, ActSubagent, "Explore", "Subagente: Explore"},
		{EN, ActMCP, "github.search", "MCP: github.search"},
		{ES, ActTool, "SomeTool", "SomeTool"},
		{EN, "future", "x", "x"},
	}
	for _, c := range cases {
		if got := ActivityText(c.l, c.kind, c.target); got != c.want {
			t.Errorf("ActivityText(%s, %s, %q) = %q, want %q", c.l, c.kind, c.target, got, c.want)
		}
	}
}

func TestParseDefaultsArray(t *testing.T) {
	got := parseDefaultsArray("(\n    \"es-ES\",\n    en\n)\n")
	if len(got) != 2 || got[0] != "es-ES" || got[1] != "en" {
		t.Fatalf("got %q", got)
	}
}
