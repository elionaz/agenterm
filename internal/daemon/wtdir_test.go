package daemon

import "testing"

func TestProjectWorktreeDir(t *testing.T) {
	root := "/w"
	for dir, want := range map[string]bool{
		"/w/api-ab12":       true, // the current name
		"/w/old-name-ab12":  true, // made before a rename
		"/w/ab12":           true, // a name with nothing to slug
		"/w/api-ab123":      false,
		"/w/xab12":          false,
		"/elsewhere/a-ab12": false,
		"/w/sub/api-ab12":   false,
	} {
		if got := projectWorktreeDir(root, dir, "ab12"); got != want {
			t.Errorf("%s: %v, want %v", dir, got, want)
		}
	}
}
