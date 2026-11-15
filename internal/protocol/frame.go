package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Attach streams use length-prefixed frames:
//
//	+--------+----------------------+-----------------+
//	| type:1 | length:4 (uint32 BE) | payload:length  |
//	+--------+----------------------+-----------------+
type FrameType byte

const (
	// Daemon -> client.
	FrameOutput     FrameType = 0x01 // raw PTY bytes
	FrameReplayDone FrameType = 0x02 // scrollback replay finished; live output follows
	FrameExit       FrameType = 0x03 // payload: int32 BE exit code; stream ends
	FrameError      FrameType = 0x04 // payload: UTF-8 message; stream ends

	// Client -> daemon.
	FrameInput  FrameType = 0x10 // raw bytes written to the PTY
	FrameResize FrameType = 0x11 // payload: cols uint16 BE, rows uint16 BE
)

// MaxFramePayload bounds a single frame; larger writes are split by callers.
const MaxFramePayload = 1 << 20

// ErrFrameTooLarge is returned when a peer announces an oversized frame.
var ErrFrameTooLarge = errors.New("protocol: frame too large")

// Frame is one decoded attach frame.
type Frame struct {
	Type    FrameType
	Payload []byte
}

// WriteFrame encodes a frame in a single Write call.
func WriteFrame(w io.Writer, t FrameType, payload []byte) error {
	if len(payload) > MaxFramePayload {
		return ErrFrameTooLarge
	}
	buf := make([]byte, 5+len(payload))
	buf[0] = byte(t)
	binary.BigEndian.PutUint32(buf[1:5], uint32(len(payload)))
	copy(buf[5:], payload)
	_, err := w.Write(buf)
	return err
}

// WriteChunked writes p as one or more frames of type t.
func WriteChunked(w io.Writer, t FrameType, p []byte) error {
	for len(p) > 0 {
		n := min(len(p), MaxFramePayload)
		if err := WriteFrame(w, t, p[:n]); err != nil {
			return err
		}
		p = p[n:]
	}
	return nil
}

// ReadFrame decodes the next frame.
func ReadFrame(r io.Reader) (Frame, error) {
	var hdr [5]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return Frame{}, err
	}
	n := binary.BigEndian.Uint32(hdr[1:5])
	if n > MaxFramePayload {
		return Frame{}, ErrFrameTooLarge
	}
	f := Frame{Type: FrameType(hdr[0]), Payload: make([]byte, n)}
	if _, err := io.ReadFull(r, f.Payload); err != nil {
		return Frame{}, fmt.Errorf("read frame payload: %w", err)
	}
	return f, nil
}

// ResizePayload encodes a resize frame payload.
func ResizePayload(cols, rows uint16) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint16(b[0:2], cols)
	binary.BigEndian.PutUint16(b[2:4], rows)
	return b
}

// ParseResize decodes a resize frame payload.
func ParseResize(p []byte) (cols, rows uint16, err error) {
	if len(p) != 4 {
		return 0, 0, fmt.Errorf("protocol: bad resize payload length %d", len(p))
	}
	return binary.BigEndian.Uint16(p[0:2]), binary.BigEndian.Uint16(p[2:4]), nil
}

// ExitPayload encodes an exit code.
func ExitPayload(code int) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(int32(code)))
	return b
}

// ParseExit decodes an exit code.
func ParseExit(p []byte) (int, error) {
	if len(p) != 4 {
		return 0, fmt.Errorf("protocol: bad exit payload length %d", len(p))
	}
	return int(int32(binary.BigEndian.Uint32(p))), nil
}
