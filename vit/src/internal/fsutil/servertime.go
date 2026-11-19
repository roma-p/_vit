package fsutil

import (
	"os"
	"time"
)

func NowAt(dir string) time.Time {
	probe := PathOS(dir, ".now-"+GenerateUID(8))
	if err := os.Mkdir(probe, 0o755); err != nil {
		return time.Now()
	}
	defer os.Remove(probe)
	info, err := os.Stat(probe)
	if err != nil {
		return time.Now()
	}
	return info.ModTime()
}

func IsStaleAt(path string, now time.Time, d time.Duration) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return false, err
	}
	return now.Sub(info.ModTime()) > d, nil
}
