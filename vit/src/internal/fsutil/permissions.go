package fsutil

import (
	"os"
)

func MakeDirectoryReadOnly(dirPath string) error {
	return os.Chmod(dirPath, 0o555)
}

func MakeDirectoryWritable(dirPath string) error {
	return os.Chmod(dirPath, 0o755)
}

func MakeFileReadOnly(filePath string) error {
	return os.Chmod(filePath, 0o444)
}

func MakeFileWritable(filePath string) error {
	return os.Chmod(filePath, 0o644)
}

func IsReadOnly(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return info.Mode().Perm()&0o200 == 0, nil
}

func IsReadWrite(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return info.Mode().Perm()&0o200 != 0, nil
}
