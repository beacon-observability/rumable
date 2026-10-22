package rumable

import (
	"os"
	"testing"
)

func TestDiscoveryRejectsRedirectedWindowsInput(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	stdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = stdin }()
	if input, err := discoveryInput(); err == nil {
		input.Close()
		t.Fatal("redirected stdin opened the console")
	}
}
