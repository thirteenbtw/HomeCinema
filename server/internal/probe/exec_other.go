//go:build !windows

package probe

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}

// HideWindow is exported for other packages that spawn ffmpeg.
func HideWindow(cmd *exec.Cmd) { hideWindow(cmd) }
