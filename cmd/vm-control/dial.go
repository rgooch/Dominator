package main

import (
	"github.com/Cloud-Foundations/Dominator/lib/srpc"
)

func dialFleetManager(address string) (*srpc.Client, error) {
	return srpc.DialHTTPWithDialer("tcp", address, rrDialer)
}

func dialHypervisor(address string) (*srpc.Client, error) {
	return srpc.DialHTTP("tcp", address, 0)
}

func dialImageServer(address string) (srpc.ClientI, error) {
	return srpc.DialHTTPWithDialer("tcp", address, rrDialer)
}
