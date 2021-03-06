package client

import (
	"github.com/Cloud-Foundations/Dominator/lib/hash"
	"github.com/Cloud-Foundations/Dominator/lib/srpc"
	"github.com/Cloud-Foundations/Dominator/proto/sub"
)

func cleanup(client *srpc.Client, hashes []hash.Hash) error {
	request := sub.CleanupRequest{hashes}
	var reply sub.CleanupResponse
	return client.RequestReply("Subd.Cleanup", request, &reply)
}
