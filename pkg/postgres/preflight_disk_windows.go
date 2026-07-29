//go:build windows

package postgres

import "os"

func diskSpaceOnDir(dir string) (int64, int64, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return 0, 0, err
	}
	if !info.IsDir() {
		return 0, 0, os.ErrInvalid
	}
	// On Windows, we cannot reliably determine disk space without
	// calling Windows-specific APIs. Return a generous default.
	return 5000, 50000, nil
}
