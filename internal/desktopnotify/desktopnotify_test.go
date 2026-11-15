package desktopnotify

import "testing"

func TestWording(t *testing.T) {
	if got := Title("web", "claude"); got != "web · claude" {
		t.Fatal(got)
	}
	if got := Title("", ""); got != "agenterm" {
		t.Fatal(got)
	}
	if Body("permission", "") != "Necesita tu aprobación" || Body("done", "") != "Terminó" || Body("done", "x") != "x" {
		t.Fatal("body")
	}
}

func TestAppleScriptString(t *testing.T) {
	if got := appleScriptString("say \"hi\"\n\\o/"); got != `"say \"hi\" \\o/"` {
		t.Fatalf("got %s", got)
	}
}
