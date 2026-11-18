//go:build !darwin && !linux

package peercred

// Parent returns pid's parent pid (via ps).
func Parent(pid int) (int, error) {
	p, err := Lookup(pid)
	return p.PPID, err
}
