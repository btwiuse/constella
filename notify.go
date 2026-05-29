package constella

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	ma "github.com/multiformats/go-multiaddr"
)

func Notify(host host.Host, relayMa ma.Multiaddr) {
	notifiee := &network.NotifyBundle{
		ListenF: func(n network.Network, a ma.Multiaddr) {
			slog.Info(
				"[Listen]",
				"ma", fmt.Sprintf("%s/p2p/%s", a, host.ID()),
				// "localAddrs", host.Addrs(),
			)
			for i, addr := range host.Addrs() {
				slog.Info("localAddr", "i", i, "addr", addr)
			}
		},
		ListenCloseF: func(n network.Network, a ma.Multiaddr) {
			slog.Info(
				"[ListenClose]",
				"ma", fmt.Sprintf("%s/p2p/%s", a, host.ID()),
				// "localAddrs", host.Addrs(),
			)
			for i, addr := range host.Addrs() {
				slog.Info("localAddr", "i", i, "addr", addr)
			}
			for i := 0; ; i++ {
				err := n.Listen(relayMa)
				if err == nil {
					break
				}
				slog.Warn("retry listen", "error", err, "delay", i)
				time.Sleep(time.Duration(i) * time.Second)
			}
		},
		ConnectedF: func(n network.Network, c network.Conn) {
			slog.Info(
				"[Connected]",
				"connId", c.ID(),
				"connRemotePeerId", c.RemotePeer(),
				"direction", c.Stat().Direction.String(),
				// "opened", c.Stat().Opened,
				// "peers", host.Peerstore().Peers(),
				// "connLocalPeerId", c.LocalPeer(),
				// "connLocalMa", c.LocalMultiaddr(),
				"connRemoteMa", c.RemoteMultiaddr(),
			)
			UpdateUniquePeers(host)
			slog.Info("peer count", "total", len(host.Peerstore().Peers()), "unique", CountUniquePeers())
		},
		DisconnectedF: func(n network.Network, c network.Conn) {
			slog.Info(
				"[Disconnected]",
				"connId", c.ID(),
				"connRemotePeerId", c.RemotePeer(),
				"direction", c.Stat().Direction.String(),
				"duration", time.Since(c.Stat().Opened),
				// "opened", c.Stat().Opened,
				// "peers", host.Peerstore().Peers(),
				// "connLocalPeerId", c.LocalPeer(),
				// "connLocalMa", c.LocalMultiaddr(),
				// "connRemoteMa", c.RemoteMultiaddr(),
			)
			UpdateUniquePeers(host)
			slog.Info("peer count", "total", len(host.Peerstore().Peers()), "unique", CountUniquePeers())
		},
	}

	host.Network().Notify(notifiee)
}

var UniquePeers = sync.Map{}

func UpdateUniquePeers(host host.Host) {
	for _, peer := range host.Peerstore().Peers() {
		// key: peer.ID, value: struct{}
		_, loaded := UniquePeers.LoadOrStore(peer, struct{}{})
		if !loaded {
			slog.Info("new peer", "peer", peer)
		}
	}
}

func CountUniquePeers() int {
	count := 0
	UniquePeers.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}
