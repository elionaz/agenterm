package main

import "testing"

func TestFeedLatest(t *testing.T) {
	xml := `<rss><channel>
<item><sparkle:version>0.11.0</sparkle:version><sparkle:shortVersionString>0.11.0</sparkle:shortVersionString></item>
<item><sparkle:shortVersionString> 0.12.0 </sparkle:shortVersionString></item>
<item><sparkle:shortVersionString>0.9.10</sparkle:shortVersionString></item>
</channel></rss>`
	if got := feedLatest(xml); got != "0.12.0" {
		t.Fatalf("feedLatest = %q", got)
	}
	if feedLatest("<rss/>") != "" {
		t.Fatal("empty feed")
	}
}

func TestNewerVersion(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"0.12.0", "0.11.9", true},
		{"0.10.0", "0.9.10", true},
		{"0.12.0", "0.12.0", false},
		{"0.11.0", "0.12.0", false},
		{"1.0.0", "dev", false},
		{"v0.13.0", "0.12.1", true},
	} {
		if got := newerVersion(c.a, c.b); got != c.want {
			t.Errorf("newerVersion(%q, %q) = %v", c.a, c.b, got)
		}
	}
}
