//go:build !windows

package rumable

import (
	"io"
	"os"
)

// The shell installer arrives on stdin. Choices must use its controlling tty.
func discoveryInput() (io.ReadCloser, error) { return os.Open("/dev/tty") }
