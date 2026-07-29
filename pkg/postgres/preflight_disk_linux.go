//go:build linux

package postgres

import "golang.org/x/sys/unix"

func diskSpaceOnDir(dir string) (int64, int64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(dir, &stat); err != nil {
		return 0, 0, err
	}
	availMB := int64(stat.Bavail) * stat.Bsize / 1024 / 1024 // #nosec G115 -- stat values always fit in int64 for real filesystems
	totalMB := int64(stat.Blocks) * stat.Bsize / 1024 / 1024 // #nosec G115 -- stat values always fit in int64 for real filesystems
	return availMB, totalMB, nil
}
