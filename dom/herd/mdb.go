package herd

import (
	"reflect"
	"time"

	filegenclient "github.com/Cloud-Foundations/Dominator/lib/filegen/client"
	"github.com/Cloud-Foundations/Dominator/lib/format"
	"github.com/Cloud-Foundations/Dominator/lib/mdb"
	"github.com/Cloud-Foundations/Dominator/lib/srpc"
)

func (herd *Herd) mdbUpdate(mdb *mdb.Mdb) {
	herd.logger.Printf("MDB data received: %d subs\n", len(mdb.Machines))
	startTime := time.Now()
	numNew, numDeleted, numChanged, wantedImages, clientResourcesToDelete :=
		herd.mdbUpdateGetLock(mdb)
	// Closing resources can lead to a release/grab cycle, so need to grab the
	// CPU to ensure we don't trigger the release leakage detector and panic.
	herd.cpuSharer.GrabCpu()
	for _, clientResource := range clientResourcesToDelete {
		clientResource.ScheduleClose()
	}
	herd.cpuSharer.ReleaseCpu()
	// Clean up unreferenced images.
	herd.imageManager.SetImageInterestList(wantedImages, true)
	pluralNew := "s"
	if numNew == 1 {
		pluralNew = ""
	}
	pluralDeleted := "s"
	if numDeleted == 1 {
		pluralDeleted = ""
	}
	pluralChanged := "s"
	if numChanged == 1 {
		pluralChanged = ""
	}
	herd.logger.Printf(
		"MDB update: %d new sub%s, %d removed sub%s, %d changed sub%s in %s",
		numNew, pluralNew, numDeleted, pluralDeleted, numChanged, pluralChanged,
		format.Duration(time.Since(startTime)))
}

func (herd *Herd) mdbUpdateGetLock(mdb *mdb.Mdb) (
	int, int, int, map[string]struct{}, []*srpc.ClientResource) {
	herd.LockWithTimeout(time.Minute)
	defer herd.Unlock()
	startTime := time.Now()
	numNew := 0
	numDeleted := 0
	numChanged := 0
	herd.subsByIndex = make([]*Sub, 0, len(mdb.Machines))
	// Mark for delete all current subs, then later unmark ones in the new MDB.
	subsToDelete := make(map[string]struct{})
	for _, sub := range herd.subsByName {
		subsToDelete[sub.mdb.Hostname] = struct{}{}
	}
	wantedImages := make(map[string]struct{})
	wantedImages[herd.defaultImageName] = struct{}{}
	wantedImages[herd.nextDefaultImageName] = struct{}{}
	for _, machine := range mdb.Machines { // Sorted by Hostname.
		if machine.Hostname == "" {
			herd.logger.Printf("Empty Hostname field, ignoring \"%s\"\n",
				machine)
			continue
		}
		sub := herd.subsByName[machine.Hostname]
		wantedImages[machine.RequiredImage] = struct{}{}
		wantedImages[machine.PlannedImage] = struct{}{}
		img := herd.imageManager.GetNoError(machine.RequiredImage)
		if sub == nil {
			sub = &Sub{
				herd:          herd,
				mdb:           machine,
				cancelChannel: make(chan struct{}),
			}
			herd.subsByName[machine.Hostname] = sub
			sub.fileUpdateReceiver =
				herd.computedFilesManager.AddAndGetReceiver(
					filegenclient.Machine{machine, sub.getComputedFiles(img)})
			numNew++
		} else {
			if sub.mdb.RequiredImage != machine.RequiredImage {
				if sub.status == statusSynced {
					sub.status = statusWaitingToPoll
				}
			}
			if !reflect.DeepEqual(sub.mdb, machine) {
				sub.mdb = machine
				sub.generationCount = 0 // Force a full poll.
				herd.computedFilesManager.Update(
					filegenclient.Machine{machine, sub.getComputedFiles(img)})
				sub.sendCancel()
				numChanged++
			}
		}
		delete(subsToDelete, machine.Hostname)
		herd.subsByIndex = append(herd.subsByIndex, sub)
		img = herd.imageManager.GetNoError(machine.PlannedImage)
		if img == nil {
			sub.havePlannedImage = false
		} else {
			sub.havePlannedImage = true
		}
	}
	delete(wantedImages, "")
	// Delete flagged subs (those not in the new MDB).
	clientResourcesToDelete := make([]*srpc.ClientResource, 0)
	for subHostname := range subsToDelete {
		sub := herd.subsByName[subHostname]
		sub.deletingFlagMutex.Lock()
		sub.deleting = true
		if sub.clientResource != nil {
			clientResourcesToDelete = append(clientResourcesToDelete,
				sub.clientResource)
		}
		sub.deletingFlagMutex.Unlock()
		herd.computedFilesManager.Remove(subHostname)
		delete(herd.subsByName, subHostname)
		herd.eraseSubFromInstallerQueue(subHostname)
		numDeleted++
	}
	mdbUpdateTimeDistribution.Add(time.Since(startTime))
	return numNew, numDeleted, numChanged, wantedImages, clientResourcesToDelete
}
