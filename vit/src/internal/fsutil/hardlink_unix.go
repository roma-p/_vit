//go:build !windows
// +build !windows

package fsutil

func CheckHardlinkSupport() error {
	return nil
}
