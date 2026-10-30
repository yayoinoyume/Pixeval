//go:build linux

package extensions

import (
	"os"
	"os/exec"
	"syscall"
)

// prepareProcess puts the extension in its own process group, so stopping a
// wrapper script kills the children it started too.
func prepareProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcess kills the extension's whole process group on Linux. If the group
// is already gone, it falls back to the direct process.
func killProcess(p *os.Process) {
	if p == nil {
		return
	}
	if err := syscall.Kill(-p.Pid, syscall.SIGKILL); err == nil {
		return
	}
	_ = p.Kill()
}
