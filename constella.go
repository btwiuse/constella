package constella

import (
	"cmp"
	"context"
	"encoding/json"
	"expvar"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"time"

	"github.com/btwiuse/dispatcher"
	"github.com/btwiuse/p2pid"
	"github.com/btwiuse/wsport"
	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/core/routing"
	p2phttp "github.com/libp2p/go-libp2p/p2p/http"
	"github.com/libp2p/go-libp2p/p2p/net/gostream"
	quic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	webtransport "github.com/libp2p/go-libp2p/p2p/transport/webtransport"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/webteleport/utils"
)

// New creates a new Constella instance.
func New(relayURL string) (*Constella, error) {
	var rout *dht.IpfsDHT
	host, err := libp2p.New(
		p2pid.FromEnv(p2pid.PID_SEED),
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Transport(quic.NewTransport),
		libp2p.Transport(webtransport.New),
		libp2p.Transport(wsport.New),
		// wsport.ListenAddrStrings(relay),
		libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
			d, err := dht.New(
				context.Background(),
				h,
				dht.Mode(dht.ModeAutoServer),
				dht.V1ProtocolOverride(
					cmp.Or(
						protocol.ID(os.Getenv("DHT")),
						dht.ProtocolDHT,
					),
				),
			)
			rout = d
			return d, err
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("libp2p.New: %w", err)
	}

	relayMa, err := wsport.FromString(relayURL)
	if err != nil {
		return nil, fmt.Errorf("wsport.FromString: %w", err)
	}

	Notify(host, relayMa)

	if err := host.Network().Listen(relayMa); err != nil {
		return nil, fmt.Errorf("Listen: %w", err)
	}

	ps, err := pubsub.NewGossipSub(context.Background(), host)
	if err != nil {
		return nil, fmt.Errorf("pubsub.NewGossipSub: %w", err)
	}

	counter, err := NewCounter(host.ID(), ps)
	if err != nil {
		return nil, fmt.Errorf("NewCounter: %w", err)
	}

	host.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(_ network.Network, _ network.Conn) {
			counter.Broadcast()
		},
	})

	return &Constella{
		Host:    host,
		Rout:    rout,
		Counter: counter,
	}, nil
}

// Constella is both a http.Handler and a libp2p.Host.
type Constella struct {
	host.Host
	Rout    *dht.IpfsDHT
	Counter *Counter
}

type Info struct {
	ID            peer.ID              `json:"id"`
	Addrs         []ma.Multiaddr       `json:"addrs"`
	Peers         []peer.ID            `json:"peers"`
	Conns         map[string]ConnStats `json:"conns"`
	Connectedness map[string]string    `json:"connectedness"`
	AddrInfos     []peer.AddrInfo      `json:"addrInfos"`
	Protocols     []protocol.ID        `json:"protocols"`
	Routing       *RoutingInfo         `json:"routing,omitempty"`
}

type RoutingInfo struct {
	Mode       string            `json:"mode"`
	BucketSize int               `json:"bucketSize"`
	TableSize  int               `json:"tableSize"`
	Peers      []RoutingPeerInfo `json:"peers"`
}

type RoutingPeerInfo struct {
	ID                            peer.ID   `json:"id"`
	LastUsefulAt                  time.Time `json:"lastUsefulAt"`
	LastSuccessfulOutboundQueryAt time.Time `json:"lastSuccessfulOutboundQueryAt"`
	AddedAt                       time.Time `json:"addedAt"`
}

type ConnStats struct {
	Stats
	RemotePeer peer.ID                 `json:"remotePeer"`
	NumStreams int                     `json:"numStreams"`
	ConnState  network.ConnectionState `json:"connState"`
	Streams    []StreamStats           `json:"streams"`
	Protocols  []protocol.ID           `json:"protocols"`
}

type Stats struct {
	Direction string    `json:"direction"`
	Opened    time.Time `json:"opened"`
	Limited   bool      `json:"limited"`
}

type StreamStats struct {
	Stats
	ID       string      `json:"id"`
	Protocol protocol.ID `json:"protocol"`
}

func (c *Constella) Info() Info {
	info := Info{
		ID:            c.Host.ID(),
		Addrs:         c.Host.Addrs(),
		Peers:         c.Host.Peerstore().Peers(),
		Conns:         c.Conns(),
		Connectedness: c.Connectedness(),
		AddrInfos:     peerstore.AddrInfos(c.Host.Peerstore(), c.Host.Peerstore().Peers()),
		Protocols:     c.Host.Mux().Protocols(),
	}

	if c.Rout != nil {
		peers := make([]RoutingPeerInfo, 0)
		for _, pi := range c.Rout.RoutingTable().GetPeerInfos() {
			peers = append(peers, RoutingPeerInfo{
				ID:                            pi.Id,
				LastUsefulAt:                  pi.LastUsefulAt,
				LastSuccessfulOutboundQueryAt: pi.LastSuccessfulOutboundQueryAt,
				AddedAt:                       pi.AddedAt,
			})
		}
		info.Routing = &RoutingInfo{
			Mode:       modeString(c.Rout.Mode()),
			BucketSize: c.Rout.BucketSize(),
			TableSize:  c.Rout.RoutingTable().Size(),
			Peers:      peers,
		}
	}

	return info
}

func modeString(m dht.ModeOpt) string {
	switch m {
	case dht.ModeAuto:
		return "auto"
	case dht.ModeClient:
		return "client"
	case dht.ModeServer:
		return "server"
	case dht.ModeAutoServer:
		return "auto-server"
	default:
		return "unknown"
	}
}

func (c *Constella) Connectedness() map[string]string {
	connectedness := map[string]string{}
	for _, p := range c.Host.Peerstore().Peers() {
		connectedness[p.String()] = c.Host.Network().Connectedness(p).String()
	}
	return connectedness
}

func (c *Constella) Conns() map[string]ConnStats {
	conns := map[string]ConnStats{}
	for _, conn := range c.Host.Network().Conns() {
		connStat := conn.Stat()
		connStats := ConnStats{
			Stats:      Stats{Direction: connStat.Direction.String(), Opened: connStat.Opened, Limited: connStat.Limited},
			NumStreams: connStat.NumStreams,
			RemotePeer: conn.RemotePeer(),
			ConnState:  conn.ConnState(),
		}
		for _, stream := range conn.GetStreams() {
			streamStat := stream.Stat()
			connStats.Streams = append(connStats.Streams, StreamStats{
				Stats:    Stats{Direction: streamStat.Direction.String(), Opened: streamStat.Opened, Limited: streamStat.Limited},
				ID:       stream.ID(),
				Protocol: stream.Protocol(),
			})
		}
		protocols, err := c.Host.Peerstore().GetProtocols(connStats.RemotePeer)
		if err != nil {
			slog.Warn("get protocols", "error", err)
		} else {
			connStats.Protocols = protocols
		}
		conns[conn.ID()] = connStats
	}
	return conns
}

func (c *Constella) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	dispatcher.DispatcherFunc(c.Dispatch).ServeHTTP(w, r)
}

func (c *Constella) Dispatch(r *http.Request) http.Handler {
	// the /http/<pid>/... endpoint is used to proxy HTTP requests to other peers
	// via the /http/1.1 protocol defined in p2phttp
	if strings.HasPrefix(r.URL.Path, "/http") {
		return http.HandlerFunc(c.HandleHTTP)
	}
	// the /term/<pid>/... endpoint is used to open a terminal to another peer
	if strings.HasPrefix(r.URL.Path, "/term") {
		return http.HandlerFunc(c.HandleTerm)
	}
	// the /add/<maddr> endpoint is used to add a new address to the peerstore
	if strings.HasPrefix(r.URL.Path, "/add") {
		return http.HandlerFunc(c.HandleAdd)
	}
	// the /counter/add endpoint increments the local CRDT counter
	if r.URL.Path == "/counter/add" {
		return http.HandlerFunc(c.HandleCounterAdd)
	}
	// the /counter endpoint returns the current counter value
	if r.URL.Path == "/counter" {
		return http.HandlerFunc(c.HandleCounter)
	}
	// the /debug/vars endpoint is used to expose expvar debug values
	if strings.HasPrefix(r.URL.Path, "/debug/vars") {
		return expvar.Handler()
	}
	// otherwise, return the JSON representation of the peer's info
	return http.HandlerFunc(c.HandleInfo)
}

func (c *Constella) HandleCounter(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	data, _ := json.MarshalIndent(map[string]any{
		"self":   c.Host.ID().String(),
		"counts": c.Counter.Snapshot(),
		"total":  c.Counter.Value(),
	}, "", "  ")
	w.Write(data)
}

func (c *Constella) HandleCounterAdd(w http.ResponseWriter, r *http.Request) {
	c.Counter.Increment()
	w.Header().Set("Content-Type", "application/json")
	data, _ := json.MarshalIndent(map[string]any{
		"self":   c.Host.ID().String(),
		"counts": c.Counter.Snapshot(),
		"total":  c.Counter.Value(),
	}, "", "  ")
	w.Write(data)
}

func (c *Constella) HandleTerm(w http.ResponseWriter, r *http.Request) {
	var pid peer.ID
	for _, peer := range c.Info().Peers {
		pfx := "/term/" + peer.String()
		if strings.HasPrefix(r.URL.Path, pfx) {
			pid = peer
			break
		}
	}
	if pid == "" {
		http.Error(w, "peer not found", http.StatusNotFound)
		return
	}
	pfx := "/term/" + pid.String()
	dialCtx := func(ctx context.Context, network, addr string) (net.Conn, error) {
		// protocol id: /term/1.0.0
		return gostream.Dial(ctx, c.Host, pid, protocol.ID("/term/1.0.0"))
	}
	var rt http.RoundTripper = &http.Transport{
		DialContext:     dialCtx,
		MaxIdleConns:    100,
		IdleConnTimeout: 90 * time.Second,
	}
	rp := utils.LoggedReverseProxy(rt)
	rp.Rewrite = func(req *httputil.ProxyRequest) {
		req.SetXForwarded()

		req.Out.URL.Host = r.Host
		req.Out.URL.Scheme = "http"
	}
	http.StripPrefix(pfx, rp).ServeHTTP(w, r)
}

func (c *Constella) HandleHTTP(w http.ResponseWriter, r *http.Request) {
	var pid peer.ID
	for _, peer := range c.Info().Peers {
		pfx := "/http/" + peer.String()
		if strings.HasPrefix(r.URL.Path, pfx) {
			pid = peer
			break
		}
	}
	if pid == "" {
		http.Error(w, "peer not found", http.StatusNotFound)
		return
	}
	pfx := "/http/" + pid.String()
	dialCtx := func(ctx context.Context, network, addr string) (net.Conn, error) {
		// protocol id: /http/1.1
		return gostream.Dial(ctx, c.Host, pid, p2phttp.ProtocolIDForMultistreamSelect)
	}
	var rt http.RoundTripper = &http.Transport{
		DialContext:     dialCtx,
		MaxIdleConns:    100,
		IdleConnTimeout: 90 * time.Second,
	}
	rp := utils.LoggedReverseProxy(rt)
	rp.Rewrite = func(req *httputil.ProxyRequest) {
		req.SetXForwarded()

		req.Out.URL.Host = r.Host
		req.Out.URL.Scheme = "http"
	}
	http.StripPrefix(pfx, rp).ServeHTTP(w, r)
}

func (c *Constella) HandleAdd(w http.ResponseWriter, r *http.Request) {
	addr := strings.TrimPrefix(r.URL.Path, "/add")
	maddr, err := ma.NewMultiaddr(addr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, err = addAddrToPeerstore(c.Host, maddr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	slog.Info("added", "addr", maddr)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (c *Constella) HandleInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	err := encoder.Encode(c.Info())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func KeepBootnodes(host host.Host, addrs []string) {
	for {
		err := ConnectBootnodes(host, addrs)
		if err != nil {
			slog.Warn("KeepBootnodes", "error", err)
		}
		time.Sleep(5 * time.Second)
	}
}

func ConnectBootnodes(host host.Host, addrs []string) error {
	for _, peerAddr := range addrs {
		peerMa, err := ma.NewMultiaddr(peerAddr)
		if err != nil {
			return err
		}

		peerInfo, err := AddrInfo(peerMa)
		if err != nil {
			return err
		}

		if host.Network().Connectedness(peerInfo.ID) == network.Connected {
			continue
		}

		slog.Info("Connecting to bootstrap", "peer", peerInfo.ID)

		err = host.Connect(context.Background(), *peerInfo)
		if err != nil {
			return err
		}
	}
	return nil
}
