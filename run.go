package constella

import (
	"cmp"
	"fmt"
	"os"

	"github.com/webteleport/wtf"
)

var (
	HTTP_RELAY = cmp.Or(os.Getenv("HTTP_RELAY"), "https://example.com")
	HTTP_PATH  = cmp.Or(os.Getenv("HTTP_PATH"), "/")
	P2P_RELAY  = cmp.Or(os.Getenv("P2P_RELAY"), "https://example.com")
	P2P_PATH   = cmp.Or(os.Getenv("P2P_PATH"), "/")
)

func Run(args []string) error {
	p2pRelay := fmt.Sprintf("%s%s", P2P_RELAY, P2P_PATH)

	c, err := New(p2pRelay)
	if err != nil {
		return fmt.Errorf("New: %w", err)
	}

	go KeepBootnodes(c, args)

	httpRelay := fmt.Sprintf("%s%s?persist=1", HTTP_RELAY, HTTP_PATH)

	return wtf.Serve(httpRelay, c)
}
