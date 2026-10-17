//go:build !windows && !linux

package migrate

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// freeBytes reports the bytes an unprivileged process can still allocate on
// the filesystem holding path.
func freeBytes(
	path string,
) (uint64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, fmt.Errorf("migrate: free space of %s: %w", path, err)
	}
	if stat.Bsize <= 0 {
		return 0, fmt.Errorf("migrate: free space of %s: block size %d is unusable", path, stat.Bsize)
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}
