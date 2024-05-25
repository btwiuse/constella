package constella

import (
	"context"
	"errors"
	"fmt"

	// We need to import libp2p's libraries that we use in this project.
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/sec"
	"github.com/libp2p/go-libp2p/p2p/net/swarm"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/btwiuse/wsport"
)

// addAddrToPeerstore parses a peer multiaddress and adds
// it to the given host's peerstore, so it knows how to
// contact it. It returns the peer ID of the remote peer.
func addAddrToPeerstore(h host.Host, addr ma.Multiaddr) (peer.ID, error) {
	addrinfo, err := AddrInfo(addr)
	if err != nil {
		return "", err
	}

	for _, addr := range addrinfo.Addrs {
		h.Peerstore().AddAddr(addrinfo.ID, addr, peerstore.PermanentAddrTTL)
	}

	return addrinfo.ID, nil
}

func RandID() peer.ID {
	host, _ := libp2p.New(
		libp2p.Transport(wsport.New),
	)
	defer host.Close()
	return host.ID()
}

// automatically add /p2p/<peerID> to the multiaddr if it is missing
// this is a workaround for the issue that AddrInfoFromP2pAddr does not work without /p2p/<peerID>
// https://github.com/libp2p/go-libp2p/issues/1040
func AddrInfo(addr ma.Multiaddr) (*peer.AddrInfo, error) {
	if _, err := addr.ValueForProtocol(ma.P_P2P); err == nil {
		return peer.AddrInfoFromP2pAddr(addr)
	}

	host, err := libp2p.New(
		libp2p.Transport(wsport.New),
	)
	if err != nil {
		return nil, err
	}
	defer host.Close()

	p2pID := ma.StringCast(fmt.Sprintf("/p2p/%s", RandID()))
	addr = addr.Encapsulate(p2pID)

	addrInfo, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return nil, err
	}

	err = host.Connect(context.Background(), *addrInfo)
	secerr := AsErrPeerIDMismatch(err)
	if secerr == nil {
		return nil, err
	}
	addrInfo.ID = secerr.Actual
	return addrInfo, nil
}

func AsErrPeerIDMismatch(err error) *sec.ErrPeerIDMismatch {
	var dialerr *swarm.DialError
	if !errors.As(err, &dialerr) {
		return nil
	}

	var mis sec.ErrPeerIDMismatch
	for _, te := range dialerr.DialErrors {
		if errors.As(te.Cause, &mis) {
			return &mis
		}
	}

	return nil
}
