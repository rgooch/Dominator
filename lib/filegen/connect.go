package filegen

import (
	"errors"
	"io"
	"io/ioutil"
	"time"

	"github.com/Cloud-Foundations/Dominator/lib/mdb"
	"github.com/Cloud-Foundations/Dominator/lib/srpc"
	proto "github.com/Cloud-Foundations/Dominator/proto/filegenerator"
)

const generateFailureRetryInterval = 15 * time.Minute

func (t *rpcType) Connect(conn *srpc.Conn) error {
	return t.manager.connect(conn) // Long-lived.
}

func (m *Manager) connect(conn *srpc.Conn) error {
	defer conn.Flush()
	clientChannel := make(chan *proto.ServerMessage, 4096)
	m.rwMutex.Lock()
	m.clients[clientChannel] = clientChannel
	m.rwMutex.Unlock()
	defer func() {
		m.rwMutex.Lock()
		delete(m.clients, clientChannel)
		m.rwMutex.Unlock()
	}()
	closeNotifyChannel := make(chan struct{})
	go m.handleClientRequests(conn, clientChannel, closeNotifyChannel)
	for {
		select {
		case serverMessage := <-clientChannel:
			if err := conn.Encode(serverMessage); err != nil {
				m.logger.Printf("error encoding ServerMessage: %s\n", err)
				return err
			}
			if len(clientChannel) < 1 {
				if err := conn.Flush(); err != nil {
					m.logger.Printf("error flushing: %s\n", err)
					return err
				}
			}
		case <-closeNotifyChannel:
			return nil
		}
	}
}

func (m *Manager) handleClientRequests(decoder srpc.Decoder,
	messageChan chan<- *proto.ServerMessage,
	closeNotifyChannel chan<- struct{}) {
	for {
		if err := m.handleRequest(decoder, messageChan); err != nil {
			if err != io.EOF {
				m.logger.Println(err)
			}
			closeNotifyChannel <- struct{}{}
		}
	}
}

func (m *Manager) handleRequest(decoder srpc.Decoder,
	messageChan chan<- *proto.ServerMessage) error {
	var request proto.ClientRequest
	if err := decoder.Decode(&request); err != nil {
		if err == io.EOF {
			return err
		}
		return errors.New("error decoding ClientRequest: " + err.Error())
	}
	serverMessage := new(proto.ServerMessage)
	if request := request.YieldRequest; request != nil {
		m.updateMachineData(request.Machine)
		fileInfos := make([]proto.FileInfo, 0, len(request.Pathnames))
		for _, pathname := range request.Pathnames {
			if fileInfo, ok := m.computeFile(request.Machine, pathname); ok {
				fileInfos = append(fileInfos, fileInfo)
				m.logger.Debugf(1,
					"handleRequest(): machine: %s, path: %s, hash: %0x\n",
					request.Machine.Hostname, pathname, fileInfo.Hash)
			}
		}
		serverMessage.YieldResponse = &proto.YieldResponse{
			Hostname: request.Machine.Hostname,
			Files:    fileInfos}
	}
	if request := request.GetObjectRequest; request != nil {
		_, reader, err := m.objectServer.GetObject(request.Hash)
		if err != nil {
			return err
		} else {
			data, _ := ioutil.ReadAll(reader)
			serverMessage.GetObjectResponse = &proto.GetObjectResponse{
				Hash: request.Hash,
				Data: data}
			reader.Close()
		}
	}
	messageChan <- serverMessage
	return nil
}

func (m *Manager) updateMachineData(machine mdb.Machine) {
	m.rwMutex.Lock()
	defer m.rwMutex.Unlock()
	if oldMachine, ok := m.machineData[machine.Hostname]; !ok {
		m.machineData[machine.Hostname] = machine
		m.logger.Debugf(0, "updateMachineData(%s): added\n", machine.Hostname)
	} else if !oldMachine.Compare(machine) {
		m.machineData[machine.Hostname] = machine
		for _, pathMgr := range m.pathManagers {
			pathMgr.rwMutex.Lock()
			delete(pathMgr.machineHashes, machine.Hostname)
			pathMgr.rwMutex.Unlock()
		}
		m.logger.Debugf(0, "updateMachineData(%s): changed\n", machine.Hostname)
	}
}

func (m *Manager) computeFile(machine mdb.Machine, pathname string) (
	proto.FileInfo, bool) {
	fileInfo := proto.FileInfo{Pathname: pathname}
	m.rwMutex.RLock()
	pathMgr, ok := m.pathManagers[pathname]
	m.rwMutex.RUnlock()
	if !ok {
		m.logger.Println("no generator for: " + pathname)
		return fileInfo, false
	}
	pathMgr.rwMutex.RLock()
	fi, ok := pathMgr.machineHashes[machine.Hostname]
	pathMgr.rwMutex.RUnlock()
	if ok {
		if fi.validUntil.IsZero() || time.Now().Before(fi.validUntil) {
			fileInfo.Hash = fi.hash
			fileInfo.Length = fi.length
			return fileInfo, true
		}
	}
	hashVal, length, validUntil, err := pathMgr.generate(machine, m.logger)
	if err != nil {
		m.logger.Printf("Error generating path: %s for machine: %s: %s\n",
			pathname, machine.Hostname, err)
		m.scheduleTimer(pathname, machine.Hostname,
			time.Now().Add(generateFailureRetryInterval))
		return fileInfo, false
	}
	fileInfo.Hash = hashVal
	fileInfo.Length = length
	pathMgr.rwMutex.Lock()
	pathMgr.machineHashes[machine.Hostname] = expiringHash{
		hashVal, length, validUntil}
	pathMgr.rwMutex.Unlock()
	m.scheduleTimer(pathname, machine.Hostname, validUntil)
	return fileInfo, true
}
