//go:build !windows

package runfile

// Unix replaces and opens a file that another process has open, so no
// error is worth a second try.
func busy(error) bool { return false }
