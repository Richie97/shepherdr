//go:build windows

package core

import (
	"os/exec"
	"syscall"
)

// createNoWindow keeps `hc connect` from flashing a console window when the
// bridge runs without one (scheduled task, -H=windowsgui build).
const createNoWindow = 0x08000000

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
}
