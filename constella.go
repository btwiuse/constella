package constella

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/btwiuse/dispatcher"
	"github.com/btwiuse/wsport"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	p2phttp "github.com/libp2p/go-libp2p/p2p/http"
	"github.com/libp2p/go-libp2p/p2p/net/gostream"
	quic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	webtransport "github.com/libp2p/go-libp2p/p2p/transport/webtransport"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/webteleport/utils"
)

// New creates a new Constella instance.
func New(relayURL string) *Constella {
	host, _ := libp2p.New(
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Transport(quic.NewTransport),
		libp2p.Transport(webtransport.New),
		libp2p.Transport(wsport.New),
		// wsport.ListenAddrStrings(relay),
	)

	relayMa, err := wsport.FromString(relayURL)
	if err != nil {
		panic(err)
	}

	Notify(host, relayMa)

	host.Network().Listen(relayMa)

	return &Constella{
		Host: host,
	}
}

// Constella is both a http.Handler and a libp2p.Host.
type Constella struct {
	host.Host
}

type Info struct {
	ID            peer.ID              `json:"id"`
	Addrs         []ma.Multiaddr       `json:"addrs"`
	Peers         []peer.ID            `json:"peers"`
	Conns         map[string]ConnStats `json:"conns"`
	Connectedness map[string]string    `json:"connectedness"`
	AddrInfos     []peer.AddrInfo      `json:"addrInfos"`
	Protocols     []protocol.ID        `json:"protocols"`
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
	return Info{
		ID:            c.Host.ID(),
		Addrs:         c.Host.Addrs(),
		Peers:         c.Host.Peerstore().Peers(),
		Conns:         c.Conns(),
		Connectedness: c.Connectedness(),
		AddrInfos:     peerstore.AddrInfos(c.Host.Peerstore(), c.Host.Peerstore().Peers()),
		Protocols:     c.Host.Mux().Protocols(),
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
			log.Println(err)
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
	// otherwise, return the JSON representation of the peer's info
	return http.HandlerFunc(c.HandleInfo)
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
	log.Println("added", maddr)
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
