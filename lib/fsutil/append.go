package fsutil

import (
	"errors"
	"io"
	"os"
)

func appendToFile(destFilename string, reader io.Reader, length uint64) error {
	destFile, err := os.OpenFile(destFilename, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	defer destFile.Close()
	if err := copyToWriter(destFile, destFilename, reader, length); err != nil {
		return err
	}
	return destFile.Close()
}

func appendFile(destFilename, sourceFilename string, mode os.FileMode, exclusive bool) error {
	if _, err := os.Stat(destFilename); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Dest file doesn't exist, so just copy the file
			if mode == 0 {
				var err error
				mode, err = getFilePerms(sourceFilename)
				if err != nil {
					return err
				}
			}
			return copyFile(destFilename, sourceFilename, mode, exclusive)
		}
	}
	sourceFile, err := os.Open(sourceFilename)
	if err != nil {
		return errors.New(sourceFilename + ": " + err.Error())
	}
	defer sourceFile.Close()
	// Dest file exists, so append to it
	return appendToFile(destFilename, sourceFile, 0)
}
