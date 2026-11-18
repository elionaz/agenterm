//go:build darwin

package peercred

import "golang.org/x/sys/unix"

// Parent returns pid's parent pid (sysctl kern.proc.pid: no process spawned).
func Parent(pid int) (int, error) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return 0, err
	}
	if kp.Proc.P_pid != int32(pid) {
		return 0, ErrNoProcess
	}
	return int(kp.Eproc.Ppid), nil
}
