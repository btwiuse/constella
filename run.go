package constella

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"time"

	"github.com/btwiuse/wsport"
	"github.com/libp2p/go-libp2p"
	p2phttp "github.com/libp2p/go-libp2p-http"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	quic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
	webtransport "github.com/libp2p/go-libp2p/p2p/transport/webtransport"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/webteleport/relay"
	"github.com/webteleport/wtf"
)

func New() *Constella {
	host, _ := libp2p.New(
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Transport(quic.NewTransport),
		libp2p.Transport(webtransport.New),
		libp2p.Transport(wsport.New),
		wsport.ListenAddrStrings(RELAY),
	)
	return &Constella{
		Host: host,
	}
}

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
		conns[conn.ID()] = connStats
	}
	return conns
}

func (c *Constella) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/http") {
		var pid peer.ID
		for _, peer := range c.Info().Peers {
			pfx := "/http/" + peer.String()
			log.Println(pfx)
			if strings.HasPrefix(r.URL.Path, pfx) {
				pid = peer
				log.Println("found", pid)
				break
			}
		}
		pfx := "/http/" + pid.String()
		dialCtx := func(ctx context.Context, network, addr string) (net.Conn, error) {
			stream, err := c.Host.NewStream(ctx, pid, p2phttp.DefaultP2PProtocol)
			if err != nil {
				return nil, err
			}
			return newConn(stream), nil
		}
		var rt http.RoundTripper = &http.Transport{
			DialContext:     dialCtx,
			MaxIdleConns:    100,
			IdleConnTimeout: 90 * time.Second,
		}
		rp := relay.ReverseProxy(rt)
		rp.Rewrite = func(req *httputil.ProxyRequest) {
			req.SetXForwarded()

			req.Out.URL.Host = r.Host
			req.Out.URL.Scheme = "http"
		}
		http.StripPrefix(pfx, rp).ServeHTTP(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/add") {
		addr := strings.TrimPrefix(r.URL.Path, "/add")
		maddr, err := ma.NewMultiaddr(addr)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		id, err := addAddrToPeerstore(c.Host, maddr)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Println("added", maddr, "with", id)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	err := encoder.Encode(c.Info())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

var RELAY = getEnv("RELAY", "https://example.com")

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func Run(args []string) error {
	return wtf.Serve(RELAY, New())
}
