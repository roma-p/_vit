//go:build !windows
// +build !windows

package fsutil

import (
	"os"

	"golang.org/x/sys/unix"
)

func TouchNow(path string) error {
	if err := unix.Utimes(path, nil); err != nil {
		return &os.PathError{Op: "utimes", Path: path, Err: err}
	}
	return nil
}
