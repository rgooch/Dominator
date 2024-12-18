package filegen

import (
	"github.com/Cloud-Foundations/Dominator/lib/log"
	"github.com/Cloud-Foundations/Dominator/lib/mdb"
	"github.com/Cloud-Foundations/Dominator/lib/objectserver/memory"
	"github.com/Cloud-Foundations/Dominator/lib/srpc"
	proto "github.com/Cloud-Foundations/Dominator/proto/filegenerator"
)

type rpcType struct {
	manager *Manager
}

func newManager(logger log.Logger) *Manager {
	m := new(Manager)
	m.pathManagers = make(map[string]*pathManager)
	m.machineData = make(map[string]mdb.Machine)
	m.clients = make(
		map[<-chan *proto.ServerMessage]chan<- *proto.ServerMessage)
	m.objectServer = memory.NewObjectServer()
	m.logger = logger
	m.registerMdbGeneratorForPath("/etc/mdb.json")
	srpc.RegisterNameWithOptions("FileGenerator", &rpcType{m},
		srpc.ReceiverOptions{
			PublicMethods: []string{
				"ListGenerators",
			}})
	return m
}

func (t *rpcType) ListGenerators(conn *srpc.Conn,
	request proto.ListGeneratorsRequest,
	reply *proto.ListGeneratorsResponse) error {
	reply.Pathnames = t.manager.GetRegisteredPaths()
	return nil
}
