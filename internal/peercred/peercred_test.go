package peercred

import (
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPeerPIDUnixSocket(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("no peer credentials on", runtime.GOOS)
	}
	dir, err := os.MkdirTemp("/tmp", "pc")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	ln, err := net.Listen("unix", filepath.Join(dir, "s"))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, _ := ln.Accept()
		accepted <- c
	}()
	client, err := net.Dial("unix", filepath.Join(dir, "s"))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server := <-accepted
	if server == nil {
		t.Fatal("accept failed")
	}
	defer server.Close()

	pid, err := PeerPID(server)
	if err != nil {
		t.Fatal(err)
	}
	if pid != os.Getpid() {
		t.Fatalf("peer pid = %d, want %d", pid, os.Getpid())
	}
	p, err := Lookup(pid)
	if err != nil {
		t.Fatal(err)
	}
	if p.PPID != os.Getppid() || !strings.Contains(p.Command, filepath.Base(os.Args[0])) {
		t.Fatalf("lookup = %+v (want ppid %d, command with %q)", p, os.Getppid(), filepath.Base(os.Args[0]))
	}
}

func TestPeerPIDNotUnix(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	if _, err := PeerPID(a); err != ErrUnsupported {
		t.Fatalf("got %v, want ErrUnsupported", err)
	}
}

func TestParent(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("no fast parent lookup here")
	}
	ppid, err := Parent(os.Getpid())
	if err != nil || ppid != os.Getppid() {
		t.Fatalf("Parent(self) = %d, %v; want %d", ppid, err, os.Getppid())
	}
	if _, err := Parent(1 << 30); err == nil {
		t.Fatal("Parent of a missing pid succeeded")
	}
}
