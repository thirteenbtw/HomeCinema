package probe

import (
	"os/exec"
	"syscall"
)

// hideWindow keeps ffprobe/ffmpeg from flashing a console window on Windows.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

// HideWindow is exported for other packages that spawn ffmpeg.
func HideWindow(cmd *exec.Cmd) { hideWindow(cmd) }
