package fsutil

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/Cloud-Foundations/Dominator/lib/wsyscall"
)

func copyToFile(destFilename string, perm os.FileMode, reader io.Reader,
	length uint64, exclusive bool) error {
	tmpFilename := destFilename + "~"
	flags := os.O_CREATE | os.O_WRONLY
	if exclusive {
		flags |= os.O_EXCL
	} else {
		flags |= os.O_TRUNC
	}
	destFile, err := os.OpenFile(tmpFilename, flags, perm)
	if err != nil {
		return err
	}
	defer os.Remove(tmpFilename)
	defer destFile.Close()
	var nCopied int64
	iLength := int64(length)
	if length < 1 {
		if _, err := io.Copy(destFile, reader); err != nil {
			return fmt.Errorf("error copying: %s", err)
		}
	} else {
		if nCopied, err = io.CopyN(destFile, reader, iLength); err != nil {
			return fmt.Errorf("error copying: %s", err)
		}
		if nCopied != iLength {
			return fmt.Errorf("expected length: %d, got: %d for: %s\n",
				length, nCopied, tmpFilename)
		}
	}
	if err := destFile.Close(); err != nil {
		return err
	}
	return os.Rename(tmpFilename, destFilename)
}

func copyTree(destDir, sourceDir string,
	copyFunc func(destFilename, sourceFilename string,
		mode os.FileMode) error) error {
	file, err := os.Open(sourceDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	names, err := file.Readdirnames(-1)
	file.Close()
	if err != nil {
		return err
	}
	for _, name := range names {
		sourceFilename := path.Join(sourceDir, name)
		destFilename := path.Join(destDir, name)
		var stat wsyscall.Stat_t
		if err := wsyscall.Lstat(sourceFilename, &stat); err != nil {
			return errors.New(sourceFilename + ": " + err.Error())
		}
		switch stat.Mode & wsyscall.S_IFMT {
		case wsyscall.S_IFDIR:
			if err := os.Mkdir(destFilename, DirPerms); err != nil {
				if !os.IsExist(err) {
					return err
				}
			}
			err := copyTree(destFilename, sourceFilename, copyFunc)
			if err != nil {
				return err
			}
		case wsyscall.S_IFREG:
			err := copyFunc(destFilename, sourceFilename,
				os.FileMode(stat.Mode)&os.ModePerm)
			if err != nil {
				return err
			}
		case wsyscall.S_IFLNK:
			sourceTarget, err := os.Readlink(sourceFilename)
			if err != nil {
				return errors.New(sourceFilename + ": " + err.Error())
			}
			if destTarget, err := os.Readlink(destFilename); err == nil {
				if sourceTarget == destTarget {
					continue
				}
			}
			os.Remove(destFilename)
			if err := os.Symlink(sourceTarget, destFilename); err != nil {
				return err
			}
		default:
			return errors.New("unsupported file type")
		}
	}
	return nil
}

func copyFile(destFilename, sourceFilename string, mode os.FileMode,
	exclusive bool) error {
	if mode == 0 {
		var stat wsyscall.Stat_t
		if err := wsyscall.Stat(sourceFilename, &stat); err != nil {
			return errors.New(sourceFilename + ": " + err.Error())
		}
		mode = os.FileMode(stat.Mode & wsyscall.S_IFMT)
	}
	sourceFile, err := os.Open(sourceFilename)
	if err != nil {
		return errors.New(sourceFilename + ": " + err.Error())
	}
	defer sourceFile.Close()
	return copyToFile(destFilename, mode, sourceFile, 0, exclusive)
}
