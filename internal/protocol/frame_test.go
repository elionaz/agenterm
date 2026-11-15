package protocol

import (
	"bytes"
	"errors"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	WriteFrame(&buf, FrameOutput, []byte("hi"))
	WriteFrame(&buf, FrameReplayDone, nil)
	WriteFrame(&buf, FrameResize, ResizePayload(132, 43))
	WriteFrame(&buf, FrameExit, ExitPayload(-1))

	f, _ := ReadFrame(&buf)
	if f.Type != FrameOutput || string(f.Payload) != "hi" {
		t.Fatalf("%+v", f)
	}
	if f, _ = ReadFrame(&buf); f.Type != FrameReplayDone || len(f.Payload) != 0 {
		t.Fatalf("%+v", f)
	}
	f, _ = ReadFrame(&buf)
	if c, r, err := ParseResize(f.Payload); err != nil || c != 132 || r != 43 {
		t.Fatalf("resize %d %d %v", c, r, err)
	}
	f, _ = ReadFrame(&buf)
	if code, err := ParseExit(f.Payload); err != nil || code != -1 {
		t.Fatalf("exit %d %v", code, err)
	}
}

func TestFrameLimits(t *testing.T) {
	if err := WriteFrame(&bytes.Buffer{}, FrameInput, make([]byte, MaxFramePayload+1)); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatal(err)
	}
	hdr := []byte{byte(FrameOutput), 0xff, 0xff, 0xff, 0xff}
	if _, err := ReadFrame(bytes.NewReader(hdr)); !errors.Is(err, ErrFrameTooLarge) {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	WriteChunked(&buf, FrameInput, make([]byte, MaxFramePayload+10))
	f1, _ := ReadFrame(&buf)
	f2, _ := ReadFrame(&buf)
	if len(f1.Payload) != MaxFramePayload || len(f2.Payload) != 10 {
		t.Fatalf("chunks %d %d", len(f1.Payload), len(f2.Payload))
	}
}
