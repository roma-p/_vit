//go:build windows
// +build windows

package fsutil

import "os"

func TouchNow(path string) error {
	now := NowAt(DirOS(path))
	return os.Chtimes(path, now, now)
}
