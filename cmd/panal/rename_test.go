package main

import "testing"

func TestParseRename(t *testing.T) {
	for _, tc := range []struct {
		args     []string
		id, name string
		ok       bool
	}{
		{[]string{"ab12", "api"}, "ab12", "api", true},
		{[]string{"ab12", "api", "server"}, "ab12", "api server", true},
		{[]string{"ab12", "--", "--weird"}, "ab12", "--weird", true},
		{[]string{"ab12", "--reset"}, "ab12", "", true},
		{[]string{"ab12"}, "", "", false},
		{[]string{"ab12", "  "}, "", "", false},
		{[]string{"--reset", "ab12"}, "", "", false},
		{nil, "", "", false},
	} {
		id, name, err := parseRename(tc.args)
		if (err == nil) != tc.ok || id != tc.id || name != tc.name {
			t.Errorf("parseRename(%q) = %q, %q, %v", tc.args, id, name, err)
		}
	}
}
