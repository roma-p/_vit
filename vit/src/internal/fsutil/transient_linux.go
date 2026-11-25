package fsutil

import (
	"errors"
	"syscall"
)

// IsTransient reports an error a retry can clear: the Linux NFS client answers EREMOTEIO for a server
// error it cannot map, seen when a file is opened while another client replaces it.
func IsTransient(err error) bool {
	return errors.Is(err, syscall.EREMOTEIO)
}
