//go:build linux
// +build linux

package main

import (
	"bytes"
	"crypto/sha512"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/Cloud-Foundations/Dominator/lib/concurrent"
	"github.com/Cloud-Foundations/Dominator/lib/format"
	"github.com/Cloud-Foundations/Dominator/lib/hash"
	"github.com/Cloud-Foundations/Dominator/lib/log"
	"github.com/Cloud-Foundations/Dominator/lib/objectserver"
)

type objectsCache struct {
	bytesScanned uint64
	objects      map[hash.Hash][]byte
}

type objectsReader struct {
	cache  *objectsCache
	hashes []hash.Hash
}

type scanStateType struct {
	device          uint64
	mutex           sync.Mutex // Protect below and also objectsCache.
	foundObjects    map[hash.Hash]uint64
	requiredObjects map[hash.Hash]uint64
	scannedInodes   map[uint64]struct{}
}

func hashFile(filename string) (hash.Hash, []byte, error) {
	object, err := os.ReadFile(filename)
	if err != nil {
		return hash.Hash{}, nil, err
	}
	hasher := sha512.New()
	hasher.Write(object)
	var hashVal hash.Hash
	copy(hashVal[:], hasher.Sum(nil))
	return hashVal, object, nil
}

func (cache *objectsCache) computeMissing(
	requiredObjects map[hash.Hash]uint64) (
	map[hash.Hash]uint64, uint64, uint64) {
	var requiredBytes, presentBytes uint64
	missingObjects := make(map[hash.Hash]uint64, len(requiredObjects))
	for hashVal, requiredSize := range requiredObjects {
		requiredBytes += requiredSize
		if object, ok := cache.objects[hashVal]; ok {
			presentBytes += uint64(len(object))
		} else {
			missingObjects[hashVal] = requiredSize
		}
	}
	return missingObjects, requiredBytes, presentBytes
}

func createObjectsCache(requiredObjects map[hash.Hash]uint64,
	objGetter objectserver.ObjectsGetter, rootDevice string,
	logger log.DebugLogger) (*objectsCache, error) {
	cache := &objectsCache{objects: make(map[hash.Hash][]byte)}
	logger.Debugln(0, "scanning root")
	cache.bytesScanned = 0
	startTime := time.Now()
	if err := cache.scanRoot(requiredObjects); err != nil {
		return nil, err
	}
	duration := time.Since(startTime)
	logger.Debugf(0, "scanned root %s in %s (%s/s)\n",
		format.FormatBytes(cache.bytesScanned), format.Duration(duration),
		format.FormatBytes(
			uint64(float64(cache.bytesScanned)/duration.Seconds())))
	missingObjects, requiredBytes, presentBytes := cache.computeMissing(
		requiredObjects)
	if len(missingObjects) < 1 {
		logger.Debugln(0, "object cache already has all required objects")
		return cache, nil
	}
	logger.Debugf(0, "object cache already has %d/%d objects (%s/%s)\n",
		len(cache.objects), len(requiredObjects),
		format.FormatBytes(presentBytes), format.FormatBytes(requiredBytes))
	err := cache.findAndScanUntrusted(missingObjects, rootDevice, logger)
	if err != nil {
		return nil, err
	}
	err = cache.downloadMissing(requiredObjects, objGetter, logger)
	if err != nil {
		return nil, err
	}
	return cache, nil
}

func (cache *objectsCache) downloadMissing(requiredObjects map[hash.Hash]uint64,
	objGetter objectserver.ObjectsGetter, logger log.DebugLogger) error {
	missingObjects, _, _ := cache.computeMissing(requiredObjects)
	if len(missingObjects) < 1 {
		return nil
	}
	hashes := make([]hash.Hash, 0, len(missingObjects))
	var totalBytes uint64
	for hashVal, size := range missingObjects {
		hashes = append(hashes, hashVal)
		totalBytes += size
	}
	startTime := time.Now()
	objectsReader, err := objGetter.GetObjects(hashes)
	if err != nil {
		return err
	}
	defer objectsReader.Close()
	for _, hashVal := range hashes {
		if err := cache.getNextObject(hashVal, objectsReader); err != nil {
			return err
		}
	}
	duration := time.Since(startTime)
	logger.Debugf(0, "downloaded %d objects (%s) in %s (%s/s)\n",
		len(missingObjects), format.FormatBytes(totalBytes),
		format.Duration(duration),
		format.FormatBytes(uint64(float64(totalBytes)/duration.Seconds())))
	return nil
}

func (cache *objectsCache) findAndScanUntrusted(
	requiredObjects map[hash.Hash]uint64, rootDevice string,
	logger log.DebugLogger) error {
	if err := mount(rootDevice, *mountPoint, "ext4", logger); err != nil {
		return nil
	}
	defer syscall.Unmount(*mountPoint, 0)
	logger.Debugln(0, "scanning old root")
	cache.bytesScanned = 0
	startTime := time.Now()
	foundObjects := make(map[hash.Hash]uint64)
	err := cache.scanTree(*mountPoint, requiredObjects, foundObjects)
	if err != nil {
		return err
	}
	var requiredBytes, foundBytes uint64
	for _, size := range requiredObjects {
		requiredBytes += size
	}
	for _, size := range foundObjects {
		foundBytes += size
	}
	duration := time.Since(startTime)
	logger.Debugf(0, "scanned old root %s in %s (%s/s)\n",
		format.FormatBytes(cache.bytesScanned), format.Duration(duration),
		format.FormatBytes(
			uint64(float64(cache.bytesScanned)/duration.Seconds())))
	logger.Debugf(0, "found %d/%d objects (%s/%s) in old file-system in %s\n",
		len(foundObjects), len(requiredObjects),
		format.FormatBytes(foundBytes), format.FormatBytes(requiredBytes),
		format.Duration(duration))
	return nil
}

func (cache *objectsCache) GetObjects(hashes []hash.Hash) (
	objectserver.ObjectsReader, error) {
	return &objectsReader{cache, hashes}, nil
}

func (cache *objectsCache) getNextObject(hashVal hash.Hash,
	objectsReader objectserver.ObjectsReader) error {
	_, reader, err := objectsReader.NextObject()
	if err != nil {
		return err
	}
	if object, err := io.ReadAll(reader); err != nil {
		return err
	} else {
		cache.objects[hashVal] = object
	}
	return nil
}

func (cache *objectsCache) handleFile(scanState *scanStateType,
	filename string) error {
	if hashVal, object, err := hashFile(filename); err != nil {
		return err
	} else if size := uint64(len(object)); size < 1 {
		return nil
	} else {
		scanState.mutex.Lock()
		defer scanState.mutex.Unlock()
		cache.bytesScanned += size
		if _, ok := cache.objects[hashVal]; ok {
			return nil
		}
		if _, ok := scanState.requiredObjects[hashVal]; !ok {
			return nil
		}
		cache.objects[hashVal] = object
		if scanState.foundObjects != nil {
			scanState.foundObjects[hashVal] = size
		}
	}
	return nil
}

func (cache *objectsCache) scanRoot(
	requiredObjects map[hash.Hash]uint64) error {
	if err := cache.scanTree("/", requiredObjects, nil); err != nil {
		return err
	}
	return nil
}

func (cache *objectsCache) scanTree(topDir string,
	requiredObjects, foundObjects map[hash.Hash]uint64) error {
	var rootStat syscall.Stat_t
	if err := syscall.Lstat(topDir, &rootStat); err != nil {
		return err
	}
	scanState := &scanStateType{
		device:          rootStat.Dev,
		foundObjects:    foundObjects,
		requiredObjects: requiredObjects,
		scannedInodes:   make(map[uint64]struct{}),
	}
	return cache.walk(topDir, scanState)
}

func (cache *objectsCache) walk(dirname string,
	scanState *scanStateType) error {
	file, err := os.Open(dirname)
	if err != nil {
		return err
	}
	names, err := file.Readdirnames(-1)
	file.Close()
	if err != nil {
		return err
	}
	var directoriesToScan, filesToScan []string
	for _, name := range names {
		pathname := filepath.Join(dirname, name)
		var stat syscall.Stat_t
		err := syscall.Lstat(pathname, &stat)
		if err != nil {
			return err
		}
		if stat.Mode&syscall.S_IFMT == syscall.S_IFDIR {
			if stat.Dev != scanState.device {
				continue
			}
			directoriesToScan = append(directoriesToScan, pathname)
		} else if stat.Mode&syscall.S_IFMT == syscall.S_IFREG {
			if _, ok := scanState.scannedInodes[stat.Ino]; ok {
				continue
			}
			scanState.scannedInodes[stat.Ino] = struct{}{}
			filesToScan = append(filesToScan, pathname)
		}
	}
	concurrentState := concurrent.NewState(0)
	for _, pathname := range filesToScan {
		pathname := pathname
		err := concurrentState.GoRun(func() error {
			return cache.handleFile(scanState, pathname)
		})
		if err != nil {
			return err
		}
	}
	if err := concurrentState.Reap(); err != nil {
		return err
	}
	for _, pathname := range directoriesToScan {
		if err := cache.walk(pathname, scanState); err != nil {
			return err
		}
	}
	return nil
}

func (or *objectsReader) Close() error {
	return nil
}

func (or *objectsReader) NextObject() (uint64, io.ReadCloser, error) {
	if len(or.hashes) < 1 {
		return 0, nil, errors.New("all objects have been consumed")
	}
	hashVal := or.hashes[0]
	or.hashes = or.hashes[1:]
	if object, ok := or.cache.objects[hashVal]; !ok {
		return 0, nil, fmt.Errorf("no such object: %x", hashVal)
	} else {
		return uint64(len(object)), io.NopCloser(bytes.NewReader(object)), nil
	}
}
