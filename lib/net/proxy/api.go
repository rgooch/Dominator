package proxy

import (
	"net"
)

// Dialer defines a dialer that can be use to create connections.
type Dialer interface {
	Dial(network, address string) (net.Conn, error)
}

func NewDialer(proxy string) (Dialer, error) {
	return newDialer(proxy)
}
