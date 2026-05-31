package constella

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"

	"github.com/btwiuse/wsport"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/webteleport/webteleport"
)

var (
	RELAY     = cmp.Or(os.Getenv("RELAY"), "https://example.com")
	P2P_PATH  = cmp.Or(os.Getenv("P2P_PATH"), "/")
	P2P_RELAY = cmp.Or(os.Getenv("P2P_RELAY"), RELAY)
)

func listen() (ln net.Listener, maddr ma.Multiaddr, err error) {
	var addrStr string

	if strings.HasPrefix(RELAY, ":") {
		ln, err = net.Listen("tcp", RELAY)
		if err != nil {
			return nil, nil, fmt.Errorf("listen tcp: %w", err)
		}
		addrStr = "http://127.0.0.1" + RELAY
	} else {
		relayURL := fmt.Sprintf("%s%s", P2P_RELAY, P2P_PATH)
		ln, err = webteleport.Listen(context.Background(), relayURL)
		if err != nil {
			return nil, nil, fmt.Errorf("listen relay: %w", err)
		}
		addrStr = fmt.Sprintf("%s://%s", ln.Addr().Network(), ln.Addr())
	}

	maddr, err = wsport.FromString(addrStr)
	if err != nil {
		return nil, nil, fmt.Errorf("wsport.FromString: %w", err)
	}

	return ln, maddr, nil
}

func Run(args []string) error {
	c, err := New()
	if err != nil {
		return fmt.Errorf("New: %w", err)
	}

	maybeRegisterJS(c)

	go KeepBootnodes(c, args)

	for {
		ln, listenMa, err := listen()
		if err != nil {
			slog.Error("listen failed, retrying", "error", err)
			continue
		}

		if err := c.Serve(ln, listenMa); err != nil {
			slog.Error("serve failed, reconnecting", "error", err)
			continue
		}
	}
}
