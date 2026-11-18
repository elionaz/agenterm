package redact

import (
	"strings"
	"testing"
)

func TestString(t *testing.T) {
	cases := []struct{ in, want string }{
		{"use sk-ant-api03-abcdefghijklmnop please", "use [redacted] please"},
		{"key sk-proj-abcdefghijklmnopqrstuvwxyz", "key [redacted]"},
		{"token ghp_abcdefghijklmnopqrstuvwxyz0123", "token [redacted]"},
		{"github_pat_11ABCDEFG0123456789_abcdef", "[redacted]"},
		{"glpat-abcdefghijklmnopqrst", "[redacted]"},
		{"xoxb-1234567890-abcdef", "[redacted]"},
		{"AKIAABCDEFGHIJKLMNOP", "[redacted]"},
		{"jwt eyJhbGciOiJIUzI1.eyJzdWIiOiIxMjM0.SflKxwRJSMeKKF2QT4", "jwt [redacted]"},
		{"clone https://bob:hunter2@example.com/x.git", "clone https://bob:[redacted]@example.com/x.git"},
		{"Authorization: Bearer abcdefghijklmnop1234", "Authorization: Bearer [redacted]"},
		{"set DB_PASSWORD=hunter2 and go", "set DB_PASSWORD=[redacted] and go"},
		{`"api_key": "abc 123"`, `"api_key": [redacted]`},
		{"client_secret: s3cr3t, next", "client_secret: [redacted], next"},
		// Ordinary prose stays.
		{"fix the token refresh bug in auth.go", "fix the token refresh bug in auth.go"},
		{"the password field must be hidden", "the password field must be hidden"},
		{"see https://github.com/elionaz/agenterm", "see https://github.com/elionaz/agenterm"},
	}
	for _, c := range cases {
		if got := String(c.in); got != c.want {
			t.Errorf("String(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPrivateKey(t *testing.T) {
	in := "here:\n-----BEGIN OPENSSH PRIVATE KEY-----\nAAAA\nBBBB\n-----END OPENSSH PRIVATE KEY-----\nthanks"
	got := String(in)
	if strings.Contains(got, "AAAA") || !strings.Contains(got, "thanks") {
		t.Fatalf("got %q", got)
	}
	cut := String("-----BEGIN RSA PRIVATE KEY-----\nAAAA")
	if strings.Contains(cut, "AAAA") {
		t.Fatalf("cut key leaked: %q", cut)
	}
}
