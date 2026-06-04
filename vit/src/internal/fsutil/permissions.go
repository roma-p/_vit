package fsutil

import (
	"os"
)

// MakeDirectoryReadOnly 0555 (r-xr-xr-x) - read and execute, no write
func MakeDirectoryReadOnly(dirPath string) error {
	return os.Chmod(dirPath, 0o555)
}

// MakeDirectoryWritable 0755 (rwxr-xr-x) - full permissions for owner
func MakeDirectoryWritable(dirPath string) error {
	return os.Chmod(dirPath, 0o755)
}

// IsReadOnly checks if a directory is read-only
func IsReadOnly(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return info.Mode().Perm()&0o200 == 0, nil
}

// IsReadWrite checks if a directory is writable 
func IsReadWrite(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return info.Mode().Perm()&0o200 != 0, nil
}
