package version

import "testing"

func TestString(t *testing.T) {
	oldV, oldC := Version, Commit
	t.Cleanup(func() { Version, Commit = oldV, oldC })

	Version, Commit = "0.1.0", "abc1234"
	if got := String(); got != "0.1.0 (abc1234)" {
		t.Fatalf("String() = %q", got)
	}
	Version = "dev"
	if got := String(); got != "dev (abc1234)" {
		t.Fatalf("String() = %q", got)
	}
}
