package fsutil

import (
	"errors"
	"io/fs"
	"syscall"
)

// IsNotExist reports a path that is gone. On NFS a path removed by another client can also answer ESTALE.
func IsNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ESTALE)
}
