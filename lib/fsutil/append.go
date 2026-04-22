package fsutil

import (
	"errors"
	"io"
	"os"
)

func appendToFile(destFilename string, reader io.Reader,
	length uint64) (err error) {
	destFile, err := os.OpenFile(destFilename, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	defer func() {
		closeError := destFile.Close()
		// If our function succeeded, but the close failed,
		// return the close error instead.
		if err == nil && closeError != nil {
			err = closeError
		}
	}()
	if err := copyToWriter(destFile, destFilename, reader, length); err != nil {
		return err
	}
	return nil
}

func appendFile(destFilename, sourceFilename string, mode os.FileMode) error {
	if _, err := os.Stat(destFilename); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Dest file doesn't exist, so just copy the file.
			if mode == 0 {
				var err error
				mode, err = getFilePerms(sourceFilename)
				if err != nil {
					return err
				}
			}
			return copyFile(destFilename, sourceFilename, mode, false)
		}
	}
	sourceFile, err := os.Open(sourceFilename)
	if err != nil {
		return errors.New(sourceFilename + ": " + err.Error())
	}
	defer sourceFile.Close()
	// Dest file exists, so append to it.
	return appendToFile(destFilename, sourceFile, 0)
}
