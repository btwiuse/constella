package constella

import (
	"cmp"
	"fmt"
	"os"

	"github.com/webteleport/wtf"
)

var (
	RELAY   = cmp.Or(os.Getenv("RELAY"), "https://example.com")
	SUBPATH = cmp.Or(os.Getenv("SUBPATH"), "/")
	P2PPATH = cmp.Or(os.Getenv("P2PPATH"), "/")
)

func Run(args []string) error {
	p2pRelay := fmt.Sprintf("%s%s", RELAY, P2PPATH)

	c, err := New(p2pRelay)
	if err != nil {
		return fmt.Errorf("New: %w", err)
	}

	go KeepBootnodes(c, args)

	httpRelay := fmt.Sprintf("%s%s?persist=1", RELAY, SUBPATH)

	return wtf.Serve(httpRelay, c)
}
