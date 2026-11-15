package session

import (
	"bytes"
	"sync"
	"testing"
)

func TestRingBasic(t *testing.T) {
	r := NewRing(8)
	if got := r.Snapshot(); len(got) != 0 {
		t.Fatalf("empty snapshot = %q", got)
	}
	r.Write([]byte("abc"))
	r.Write([]byte("de"))
	if got := string(r.Snapshot()); got != "abcde" {
		t.Fatalf("got %q", got)
	}
	if r.Len() != 5 || r.Cap() != 8 {
		t.Fatalf("len=%d cap=%d", r.Len(), r.Cap())
	}
}

func TestRingWraparound(t *testing.T) {
	r := NewRing(8)
	r.Write([]byte("abcdef"))
	r.Write([]byte("ghij")) // wraps: keeps last 8
	if got := string(r.Snapshot()); got != "cdefghij" {
		t.Fatalf("got %q", got)
	}
	r.Write([]byte("k"))
	if got := string(r.Snapshot()); got != "defghijk" {
		t.Fatalf("got %q", got)
	}
	if r.Total() != 11 {
		t.Fatalf("total = %d", r.Total())
	}
}

func TestRingOversizedWrite(t *testing.T) {
	r := NewRing(4)
	r.Write([]byte("xy"))
	r.Write([]byte("0123456789"))
	if got := string(r.Snapshot()); got != "6789" {
		t.Fatalf("got %q", got)
	}
	r.Write([]byte("ab"))
	if got := string(r.Snapshot()); got != "89ab" {
		t.Fatalf("got %q", got)
	}
}

func TestRingExactFill(t *testing.T) {
	r := NewRing(4)
	r.Write([]byte("ab"))
	r.Write([]byte("cd"))
	if got := string(r.Snapshot()); got != "abcd" {
		t.Fatalf("got %q", got)
	}
}

func TestRingSnapshotIsCopy(t *testing.T) {
	r := NewRing(4)
	r.Write([]byte("abcd"))
	s := r.Snapshot()
	r.Write([]byte("zz"))
	if string(s) != "abcd" {
		t.Fatalf("snapshot mutated: %q", s)
	}
}

func TestRingMatchesReference(t *testing.T) {
	r := NewRing(37)
	var ref []byte
	for i := 0; i < 200; i++ {
		chunk := bytes.Repeat([]byte{byte('a' + i%26)}, i%11)
		r.Write(chunk)
		ref = append(ref, chunk...)
		want := ref
		if len(want) > 37 {
			want = want[len(want)-37:]
		}
		if got := r.Snapshot(); !bytes.Equal(got, want) {
			t.Fatalf("step %d: got %q want %q", i, got, want)
		}
	}
}

func TestRingConcurrent(t *testing.T) {
	r := NewRing(64)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				r.Write([]byte("hello"))
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				_ = r.Snapshot()
			}
		}()
	}
	wg.Wait()
	if r.Total() != 8*500*5 {
		t.Fatalf("total = %d", r.Total())
	}
}

func TestRingDefaultSize(t *testing.T) {
	if NewRing(0).Cap() != DefaultRingSize {
		t.Fatal("default size")
	}
}
