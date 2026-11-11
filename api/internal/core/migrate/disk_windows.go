//go:build windows

package migrate

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// freeBytes reports the bytes the calling user can still allocate on the volume
// holding path, which honours per-user quotas.
func freeBytes(
	path string,
) (uint64, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, fmt.Errorf("migrate: free space of %s: %w", path, err)
	}
	var available, total, free uint64
	if err := windows.GetDiskFreeSpaceEx(name, &available, &total, &free); err != nil {
		return 0, fmt.Errorf("migrate: free space of %s: %w", path, err)
	}
	return available, nil
}
