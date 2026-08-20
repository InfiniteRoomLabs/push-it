//go:build !windows

package hook

import (
	"os/exec"
	"syscall"
)

// detach puts the child in its own session so it outlives git.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
