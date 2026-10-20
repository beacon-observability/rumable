//go:build !linux

package rumable

import (
	"errors"
	"fmt"
	"os"
)

func attachmentIdentity(name string) (string, error) {
	i, err := os.Stat(name)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%v:%v", i.Mode(), i.ModTime()), nil
}

func replaceAttachmentFile(string, []byte, []byte, os.FileMode) error {
	return errors.New("automatic attachment requires Linux")
}
