package constella

import (
	"cmp"
	"fmt"
	"os"
	"strings"

	"github.com/webteleport/wtf"
)

var (
	RELAY      = cmp.Or(os.Getenv("RELAY"), "https://example.com")
	HTTP_RELAY = cmp.Or(os.Getenv("HTTP_RELAY"), RELAY)
	HTTP_PATH  = cmp.Or(os.Getenv("HTTP_PATH"), "/")
	P2P_RELAY  = cmp.Or(os.Getenv("P2P_RELAY"), RELAY)
	P2P_PATH   = cmp.Or(os.Getenv("P2P_PATH"), "/")
)

func Run(args []string) error {
	p2pRelay := fmt.Sprintf("%s%s", P2P_RELAY, P2P_PATH)

	c, err := New(p2pRelay)
	if err != nil {
		return fmt.Errorf("New: %w", err)
	}

	maybeRegisterJS(c)

	go KeepBootnodes(c, args)

	httpRelay := HTTP_RELAY
	if !strings.HasPrefix(httpRelay, ":") {
		httpRelay = fmt.Sprintf("%s%s?persist=1", httpRelay, HTTP_PATH)
	}

	return wtf.Serve(httpRelay, c)
}
