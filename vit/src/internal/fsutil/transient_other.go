//go:build !linux

package fsutil

func IsTransient(err error) bool {
	return false
}
