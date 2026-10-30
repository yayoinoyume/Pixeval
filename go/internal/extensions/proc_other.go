//go:build !linux

package extensions

import (
	"os"
	"os/exec"
)

// prepareProcess is a no-op: only Linux gets the process-group kill.
func prepareProcess(cmd *exec.Cmd) {}

// killProcess kills only the direct child on systems without the Linux process
// group handling. A wrapper script's children may outlive it there.
func killProcess(p *os.Process) {
	if p != nil {
		_ = p.Kill()
	}
}
