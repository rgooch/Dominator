package util

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/Cloud-Foundations/Dominator/lib/filesystem"
	"github.com/Cloud-Foundations/Dominator/lib/fsutil"
	"github.com/Cloud-Foundations/Dominator/lib/json"
)

func spliceComputedFiles(fs *filesystem.FileSystem,
	computedFileList []ComputedFile) error {
	if len(computedFileList) < 1 {
		return nil
	}
	filenameToInodeTable := fs.FilenameToInodeTable()
	inodeToFilenamesTable := fs.InodeToFilenamesTable()
	for _, computedFile := range computedFileList {
		inum, ok := filenameToInodeTable[computedFile.Filename]
		if !ok {
			return errors.New(computedFile.Filename + ": missing from image")
		}
		if filenames, ok := inodeToFilenamesTable[inum]; !ok {
			panic(computedFile.Filename + ": no corresponding list of files")
		} else if len(filenames) != 1 {
			return fmt.Errorf("%s: multiple inodes: %d", computedFile.Filename,
				len(filenames))
		}
		if inode, ok :=
			fs.InodeTable[inum].(*filesystem.ComputedRegularInode); ok {
			inode.Source = computedFile.Source
			continue
		}
		if oldInode, ok := fs.InodeTable[inum].(*filesystem.RegularInode); !ok {
			return fmt.Errorf("%s: type: %T is not a regular inode",
				computedFile.Filename, fs.InodeTable[inum])
		} else {
			newInode := new(filesystem.ComputedRegularInode)
			newInode.Mode = oldInode.Mode
			newInode.Uid = oldInode.Uid
			newInode.Gid = oldInode.Gid
			newInode.Source = computedFile.Source
			fs.InodeTable[inum] = newInode
		}
	}
	fs.ComputeTotalDataBytes()
	clearInodePointers(&fs.DirectoryInode, "")
	return fs.RebuildInodePointers()
}

func loadComputedFiles(filename string) ([]ComputedFile, error) {
	var computedFileList []ComputedFile
	if strings.HasSuffix(filename, ".json") {
		file, err := os.Open(filename)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		reader := bufio.NewReader(file)
		if err := json.Read(reader, &computedFileList); err != nil {
			return nil, errors.New("error decoding computed files list " +
				err.Error())
		}
	} else {
		lines, err := fsutil.LoadLines(filename)
		if err != nil {
			return nil, err
		}
		for _, line := range lines {
			fields := strings.Fields(line)
			if len(fields) != 2 {
				return nil, fmt.Errorf("bad line: %s", line)
			}
			computedFileList = append(computedFileList,
				ComputedFile{fields[0], fields[1]})
		}
	}
	return computedFileList, nil
}

func clearInodePointers(directoryInode *filesystem.DirectoryInode,
	name string) {
	for _, dirent := range directoryInode.EntryList {
		if inode, ok := dirent.Inode().(*filesystem.DirectoryInode); ok {
			clearInodePointers(inode, path.Join(name, dirent.Name))
		}
		dirent.SetInode(nil)
	}
}

func mergeComputedFiles(base, overlay []ComputedFile) []ComputedFile {
	computedFilesMap := make(map[string]string)
	for _, computedFile := range base {
		computedFilesMap[computedFile.Filename] = computedFile.Source
	}
	for _, computedFile := range overlay {
		computedFilesMap[computedFile.Filename] = computedFile.Source
	}
	filenames := make([]string, 0, len(computedFilesMap))
	for filename := range computedFilesMap {
		filenames = append(filenames, filename)
	}
	sort.Strings(filenames)
	newList := make([]ComputedFile, 0, len(computedFilesMap))
	for _, filename := range filenames {
		newList = append(newList, ComputedFile{
			Filename: filename,
			Source:   computedFilesMap[filename],
		})
	}
	return newList
}
