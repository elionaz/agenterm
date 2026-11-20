package desktopnotify

import (
	"testing"
	"time"

	"github.com/elionaz/panal/internal/i18n"
)

func TestWording(t *testing.T) {
	if got := Title("web", "claude"); got != "web · claude" {
		t.Fatal(got)
	}
	if got := Title("", ""); got != "panal" {
		t.Fatal(got)
	}
	if Body(i18n.ES, "permission", "") != "Necesita tu aprobación" || Body(i18n.ES, "done", "") != "Terminó" || Body(i18n.ES, "done", "x") != "x" {
		t.Fatal("body es")
	}
	if Body(i18n.EN, "permission", "") != "Needs your approval" || Body(i18n.EN, "other", "") != "Needs your attention" || Body(i18n.EN, "done", "¿Sigo?") != "¿Sigo?" {
		t.Fatal("body en")
	}
}

func TestStallBody(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	out, at := t0, t0.Add(5*time.Minute)
	if got := StallBody(i18n.EN, "Bash: sleep 999", &out, nil, &at); got != "Stalled? No activity for 5m · Bash: sleep 999" {
		t.Fatal(got)
	}
	if got := StallBody(i18n.ES, "", &out, nil, &at); got != "¿Trabado? Sin actividad hace 5m" {
		t.Fatal(got)
	}
}

func TestAppleScriptString(t *testing.T) {
	if got := appleScriptString("say \"hi\"\n\\o/"); got != `"say \"hi\" \\o/"` {
		t.Fatalf("got %s", got)
	}
}

func TestDisabled(t *testing.T) {
	for v, want := range map[string]bool{"off": true, " OFF ": true, "0": true, "false": true, "no": true, "": false, "on": false, "1": false} {
		t.Setenv(EnvNotify, v)
		if got := Disabled(); got != want {
			t.Errorf("%s=%q: Disabled() = %v, want %v", EnvNotify, v, got, want)
		}
	}
}
