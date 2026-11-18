//go:build linux

package peercred

import (
	"os"
	"strconv"
	"strings"
)

// Parent returns pid's parent pid (from /proc/<pid>/stat).
func Parent(pid int) (int, error) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, err
	}
	// pid (comm) state ppid …; comm may contain spaces and parentheses.
	s := string(b)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return 0, ErrNoProcess
	}
	f := strings.Fields(s[i+1:])
	if len(f) < 2 {
		return 0, ErrNoProcess
	}
	return strconv.Atoi(f[1])
}
