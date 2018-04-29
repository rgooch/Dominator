package manager

import (
	proto "github.com/Symantec/Dominator/proto/hypervisor"
)

func (m *Manager) closeUpdateChannel(channel <-chan proto.Update) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	delete(m.notifiers, channel)
}

func (m *Manager) makeUpdateChannel() <-chan proto.Update {
	channel := make(chan proto.Update, 16)
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.notifiers[channel] = channel
	subnets := make([]proto.Subnet, 0, len(m.subnets)-1)
	for id, subnet := range m.subnets {
		if id != "hypervisor" {
			subnets = append(subnets, subnet)
		}
	}
	vms := make(map[string]*proto.VmInfo, len(m.vms))
	for addr, vm := range m.vms {
		vms[addr] = &vm.VmInfo
	}
	channel <- proto.Update{
		HaveAddressPool: true,
		AddressPool:     m.addressPool,
		HaveSubnets:     true,
		Subnets:         subnets,
		VMs:             vms,
	}
	return channel
}

func (m *Manager) sendUpdate(update proto.Update) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.sendUpdateWithLock(update)
}

func (m *Manager) sendUpdateWithLock(update proto.Update) {
	for readChannel, writeChannel := range m.notifiers {
		select {
		case writeChannel <- update:
		default:
			close(writeChannel)
			delete(m.notifiers, readChannel)
		}
	}
}
