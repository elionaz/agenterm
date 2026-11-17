package order

import (
	"slices"
	"testing"
)

func TestFront(t *testing.T) {
	got, err := Front([]string{"a", "b", "c", "d"}, []string{"c", "a"})
	if err != nil || !slices.Equal(got, []string{"c", "a", "b", "d"}) {
		t.Fatalf("got %v %v", got, err)
	}
	if got, _ := Front([]string{"a", "b"}, nil); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("empty ids: %v", got)
	}
	for _, ids := range [][]string{{"x"}, {"a", "a"}} {
		if _, err := Front([]string{"a", "b"}, ids); err == nil {
			t.Errorf("%v: want error", ids)
		}
	}
}

func TestWithin(t *testing.T) {
	// p1's sessions are 1, 3 and 5; the others stay put.
	all := []string{"1", "2", "3", "4", "5"}
	p1 := func(id string) bool { return id == "1" || id == "3" || id == "5" }
	got, err := Within(all, p1, []string{"5", "1", "3"})
	if err != nil || !slices.Equal(got, []string{"5", "2", "1", "4", "3"}) {
		t.Fatalf("got %v %v", got, err)
	}
	if _, err := Within(all, p1, []string{"2"}); err == nil {
		t.Error("an id of another group must fail")
	}
	if !slices.Equal(all, []string{"1", "2", "3", "4", "5"}) {
		t.Error("input modified")
	}
}
