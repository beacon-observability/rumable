package rumable

import (
	"io"
	"os"
	"syscall"
)

func discoveryInput() (io.ReadCloser, error) {
	// CONIN$ can be opened even when the caller redirected stdin. Check the
	// inherited handle first, so CI/test runners cannot hang on a hidden prompt.
	var mode uint32
	if err := syscall.GetConsoleMode(syscall.Handle(os.Stdin.Fd()), &mode); err != nil {
		return nil, err
	}
	return os.Open("CONIN$")
}
