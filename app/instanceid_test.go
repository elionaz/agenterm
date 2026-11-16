package main

import "testing"

func TestInstanceID(t *testing.T) {
	if got := instanceID("", ""); got != defaultInstanceID {
		t.Errorf("no bundle, no home: %q", got)
	}
	if got := instanceID("com.elionaz.agenterm", ""); got != "com.elionaz.agenterm" {
		t.Errorf("release bundle: %q", got)
	}
	if got := instanceID("com.example.test", ""); got != "com.example.test" {
		t.Errorf("other bundle: %q", got)
	}
	a, b := instanceID("com.elionaz.agenterm", "/tmp/a"), instanceID("com.elionaz.agenterm", "/tmp/b")
	if a == b || a == "com.elionaz.agenterm" {
		t.Errorf("homes must give distinct ids: %q %q", a, b)
	}
	if a != instanceID("com.elionaz.agenterm", "/tmp/a") {
		t.Error("id must be stable for the same home")
	}
}
