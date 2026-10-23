//go:build !windows

package torrent

import "golang.org/x/sys/unix"

// FreeSpace returns free bytes on the volume holding dir.
func FreeSpace(dir string) (uint64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return 0, err
	}
	return st.Bavail * uint64(st.Bsize), nil
}
