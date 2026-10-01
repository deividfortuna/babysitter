//go:build windows

package runfile

import (
	"errors"

	"golang.org/x/sys/windows"
)

func busy(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_ACCESS_DENIED)
}
