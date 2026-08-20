//go:build windows

package hook

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// detach starts the child without a console and in its own process group.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP,
	}
}
