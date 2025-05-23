package herd

import (
	"errors"
	"flag"
	"net"
	"os"
	"runtime"
	"time"

	"github.com/Cloud-Foundations/Dominator/dom/images"
	"github.com/Cloud-Foundations/Dominator/lib/constants"
	"github.com/Cloud-Foundations/Dominator/lib/cpusharer"
	filegenclient "github.com/Cloud-Foundations/Dominator/lib/filegen/client"
	"github.com/Cloud-Foundations/Dominator/lib/log"
	libnet "github.com/Cloud-Foundations/Dominator/lib/net"
	"github.com/Cloud-Foundations/Dominator/lib/net/reverseconnection"
	"github.com/Cloud-Foundations/Dominator/lib/objectserver"
	"github.com/Cloud-Foundations/Dominator/lib/srpc"
	"github.com/Cloud-Foundations/Dominator/lib/url"
	domproto "github.com/Cloud-Foundations/Dominator/proto/dominator"
	subproto "github.com/Cloud-Foundations/Dominator/proto/sub"
	"github.com/Cloud-Foundations/tricorder/go/tricorder"
)

var (
	disableUpdatesAtStartup = flag.Bool("disableUpdatesAtStartup", false,
		"If true, updates are disabled at startup")
	pollSlotsPerCPU = flag.Uint("pollSlotsPerCPU", 100,
		"Number of poll slots per CPU")
	subConnectTimeout = flag.Uint("subConnectTimeout", 15,
		"Timeout in seconds for sub connections. If zero, OS timeout is used")
	subdInstallDelay = flag.Duration("subdInstallDelay", 5*time.Minute,
		"Time to wait before attempting to install subd")
	subdInstallRetryDelay = flag.Duration("subdInstallRetryDelay", time.Hour,
		"Time to wait before reattempting to install subd")
	subdInstaller = flag.String("subdInstaller", "",
		"Path to programme used to install subd if connections fail")
)

func newHerd(imageServerAddress string, objectServer objectserver.ObjectServer,
	metricsDir *tricorder.DirectorySpec, logger log.DebugLogger) *Herd {
	var herd Herd
	herd.imageManager = images.New(imageServerAddress, logger)
	herd.objectServer = objectServer
	herd.computedFilesManager = filegenclient.New(objectServer, logger)
	herd.logger = logger
	if *disableUpdatesAtStartup {
		herd.updatesDisabledReason = "by default"
	}
	herd.configurationForSubs.ScanExclusionList =
		constants.ScanExcludeList
	herd.subsByName = make(map[string]*Sub)
	numPollSlots := uint(runtime.NumCPU()) * *pollSlotsPerCPU
	herd.pollSemaphore = make(chan struct{}, numPollSlots)
	herd.pushSemaphore = make(chan struct{}, runtime.NumCPU())
	herd.fastUpdateSemaphore = make(chan struct{}, runtime.NumCPU())
	herd.cpuSharer = cpusharer.NewFifoCpuSharer()
	herd.cpuSharer.SetGrabTimeout(time.Minute * 15)
	herd.dialer = libnet.NewCpuSharingDialer(reverseconnection.NewDialer(
		&net.Dialer{Timeout: time.Second * time.Duration(*subConnectTimeout)},
		nil, time.Second*30, 0, logger),
		herd.cpuSharer)
	herd.currentScanStartTime = time.Now()
	herd.setupMetrics(metricsDir)
	go herd.subdInstallerLoop()
	return &herd
}

func (herd *Herd) clearSafetyShutoff(hostname string,
	authInfo *srpc.AuthInformation) error {
	herd.Lock()
	sub, ok := herd.subsByName[hostname]
	herd.Unlock()
	if !ok {
		return errors.New("unknown sub: " + hostname)
	}
	return sub.clearSafetyShutoff(authInfo)
}

func (herd *Herd) configureSubs(configuration subproto.Configuration) error {
	herd.Lock()
	defer herd.Unlock()
	herd.configurationForSubs = configuration
	return nil
}

func (herd *Herd) disableUpdates(username, reason string) error {
	if reason == "" {
		return errors.New("error disabling updates: no reason given")
	}
	herd.updatesDisabledBy = username
	herd.updatesDisabledReason = "because: " + reason
	herd.updatesDisabledTime = time.Now()
	return nil
}

func (herd *Herd) enableUpdates() error {
	herd.updatesDisabledReason = ""
	return nil
}

func (herd *Herd) fastUpdate(request domproto.FastUpdateRequest,
	authInfo *srpc.AuthInformation) (<-chan FastUpdateMessage, error) {
	if request.Timeout < time.Millisecond {
		request.Timeout = 15 * time.Minute
	}
	herd.Lock()
	sub, ok := herd.subsByName[request.Hostname]
	herd.Unlock()
	if !ok {
		return nil, errors.New("unknown sub: " + request.Hostname)
	}
	return sub.fastUpdate(request.Timeout, authInfo)
}

func (herd *Herd) forceDisruptiveUpdate(hostname string,
	authInfo *srpc.AuthInformation) error {
	herd.Lock()
	sub, ok := herd.subsByName[hostname]
	herd.Unlock()
	if !ok {
		return errors.New("unknown sub: " + hostname)
	}
	return sub.forceDisruptiveUpdate(authInfo)
}

func (herd *Herd) getSubsConfiguration() subproto.Configuration {
	herd.RLockWithTimeout(time.Minute)
	defer herd.RUnlock()
	return herd.configurationForSubs
}

func (herd *Herd) lockWithTimeout(timeout time.Duration) {
	timeoutFunction(herd.Lock, timeout)
}

func (herd *Herd) pollNextSub() bool {
	if herd.nextSubToPoll >= uint(len(herd.subsByIndex)) {
		herd.nextSubToPoll = 0
		scanDuration := time.Since(herd.currentScanStartTime)
		herd.previousScanDuration = scanDuration
		cycleTimeDistribution.Add(scanDuration)
		herd.scanCounter++
		herd.totalScanDuration += herd.previousScanDuration
		return true
	}
	if herd.nextSubToPoll == 0 {
		herd.currentScanStartTime = time.Now()
	}
	sub := herd.subsByIndex[herd.nextSubToPoll]
	herd.nextSubToPoll++
	if sub.busy { // Quick lockless check.
		return false
	}
	herd.cpuSharer.GoWhenIdle(0, -1, func() {
		if !sub.tryMakeBusy() {
			return
		}
		if sub.connectAndPoll(nil) { // Returns true if a retry is reasonable
			sub.connectAndPoll(nil)
		}
		sub.makeUnbusy()
	})
	return false
}

func (herd *Herd) countSelectedSubs(subCounters []subCounter) uint64 {
	herd.RLock()
	defer herd.RUnlock()
	if len(subCounters) < 1 {
		return uint64(len(herd.subsByIndex))
	}
	for _, sub := range herd.subsByIndex {
		for _, subCounter := range subCounters {
			if subCounter.selectFunc(sub) {
				*subCounter.counter++
			}
		}
	}
	return uint64(len(herd.subsByIndex))
}

func (herd *Herd) getSelectedSubs(selectFunc func(*Sub) bool) []*Sub {
	herd.RLock()
	defer herd.RUnlock()
	subs := make([]*Sub, 0, len(herd.subsByIndex))
	for _, sub := range herd.subsByIndex {
		if selectFunc == nil || selectFunc(sub) {
			subs = append(subs, sub)
		}
	}
	return subs
}

func (herd *Herd) getSub(name string) *Sub {
	herd.RLock()
	defer herd.RUnlock()
	return herd.subsByName[name]
}

func (herd *Herd) getReachableSelector(parsedQuery url.ParsedQuery) (
	func(*Sub) bool, error) {
	duration, err := parsedQuery.Last()
	if err != nil {
		return nil, err
	}
	return rDuration(duration).selector, nil
}

func (herd *Herd) getUnreachableSelector(parsedQuery url.ParsedQuery) (
	func(*Sub) bool, error) {
	duration, err := parsedQuery.Last()
	if err != nil {
		return nil, err
	}
	return uDuration(duration).selector, nil
}

func (herd *Herd) rLockWithTimeout(timeout time.Duration) {
	timeoutFunction(herd.RLock, timeout)
}

func (herd *Herd) setDefaultImage(imageName string) error {
	if imageName == "" {
		herd.Lock()
		defer herd.Unlock()
		herd.defaultImageName = ""
		// Cancel blocking operations by affected subs.
		for _, sub := range herd.subsByIndex {
			if sub.mdb.RequiredImage != "" {
				sub.sendCancel()
				sub.status = statusImageUndefined
			}
		}
		return nil
	}
	if imageName == herd.defaultImageName {
		return nil
	}
	herd.Lock()
	herd.nextDefaultImageName = imageName
	herd.Unlock()
	doLockedCleanup := true
	defer func() {
		if doLockedCleanup {
			herd.Lock()
			herd.nextDefaultImageName = ""
			herd.Unlock()
		}
	}()
	img, err := herd.imageManager.Get(imageName, true)
	if err != nil {
		return err
	}
	if img == nil {
		return errors.New("unknown image: " + imageName)
	}
	if img.Filter != nil {
		return errors.New("only sparse images can be set as default")
	}
	if len(img.FileSystem.InodeTable) > 100 {
		return errors.New("cannot set default image with more than 100 inodes")
	}
	doLockedCleanup = false
	herd.Lock()
	defer herd.Unlock()
	herd.defaultImageName = imageName
	herd.nextDefaultImageName = ""
	for _, sub := range herd.subsByIndex {
		if sub.mdb.RequiredImage == "" {
			sub.sendCancel()
			if sub.status == statusSynced { // Synced to previous default image.
				sub.status = statusWaitingToPoll
			}
			if sub.status == statusImageUndefined {
				sub.status = statusWaitingToPoll
			}
		}
	}
	return nil
}

func timeoutFunction(f func(), timeout time.Duration) {
	if timeout < 0 {
		f()
		return
	}
	completionChannel := make(chan struct{})
	go func() {
		f()
		completionChannel <- struct{}{}
	}()
	timer := time.NewTimer(timeout)
	select {
	case <-completionChannel:
		if !timer.Stop() {
			<-timer.C
		}
		return
	case <-timer.C:
		os.Stderr.Write([]byte("lock timeout. Full stack trace follows:\n"))
		buf := make([]byte, 1024*1024)
		nBytes := runtime.Stack(buf, true)
		os.Stderr.Write(buf[0:nBytes])
		os.Stderr.Write([]byte("\n"))
		panic("timeout")
	}
}
