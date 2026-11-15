package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"

	"github.com/elionaz/agenterm/internal/client"
	"github.com/elionaz/agenterm/internal/protocol"
)

// detachKey is Ctrl-\ (FS, 0x1c).
const detachKey = 0x1c

func termSize() (cols, rows uint16, ok bool) {
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 || h <= 0 {
		return 0, 0, false
	}
	return uint16(w), uint16(h), true
}

func attach(ctx context.Context, c *client.Client, id string) error {
	cols, rows, _ := termSize()
	st, err := c.Attach(ctx, id, cols, rows)
	if err != nil {
		return err
	}
	defer st.Close()

	stdinFd := int(os.Stdin.Fd())
	if term.IsTerminal(stdinFd) {
		old, err := term.MakeRaw(stdinFd)
		if err != nil {
			return err
		}
		defer term.Restore(stdinFd, old)
	}

	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer signal.Stop(winch)

	detached := make(chan struct{})
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				chunk := buf[:n]
				if i := bytes.IndexByte(chunk, detachKey); i >= 0 {
					if i > 0 {
						st.Input(chunk[:i])
					}
					close(detached)
					st.Close()
					return
				}
				if st.Input(chunk) != nil {
					return
				}
			}
			if err != nil {
				return // stdin EOF: keep streaming output until exit/detach
			}
		}
	}()
	go func() {
		for {
			select {
			case <-winch:
				if w, h, ok := termSize(); ok {
					st.Resize(w, h)
				}
			case <-detached:
				return
			case <-ctx.Done():
				st.Close()
				return
			}
		}
	}()

	result := recvLoop(ctx, st, id, detached)
	// \r\n because the terminal may still be in raw mode until the deferred restore.
	fmt.Fprint(os.Stderr, "\r\n"+result+"\r\n")
	return nil
}

// recvLoop copies output to stdout until the stream ends and describes why.
func recvLoop(ctx context.Context, st *client.Stream, id string, detached <-chan struct{}) string {
	for {
		f, err := st.Recv()
		if err != nil {
			select {
			case <-detached:
				return "[detached from " + id + "]"
			default:
			}
			if errors.Is(err, io.EOF) || ctx.Err() != nil {
				return "[detached from " + id + "]"
			}
			return "[connection lost: " + err.Error() + "]"
		}
		switch f.Type {
		case protocol.FrameOutput:
			os.Stdout.Write(f.Payload)
		case protocol.FrameExit:
			code, _ := protocol.ParseExit(f.Payload)
			return fmt.Sprintf("[session %s exited with code %d]", id, code)
		case protocol.FrameError:
			return "[daemon: " + string(f.Payload) + "]"
		}
	}
}
