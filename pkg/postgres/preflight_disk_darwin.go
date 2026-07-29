//go:build darwin

package postgres

import "syscall"

func diskSpaceOnDir(dir string) (int64, int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(dir, &stat); err != nil {
		return 0, 0, err
	}
	availableMB := int64(stat.Bavail) * int64(stat.Bsize) / 1024 / 1024
	totalMB := int64(stat.Blocks) * int64(stat.Bsize) / 1024 / 1024
	return availableMB, totalMB, nil
}
