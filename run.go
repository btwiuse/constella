package constella

import (
	"cmp"
	"fmt"
	"os"

	"github.com/webteleport/wtf"
)

var (
	RELAY   = cmp.Or(os.Getenv("RELAY"), "https://example.com")
	SUBPATH = cmp.Or(os.Getenv("SUBPATH"), "/constella")
)

func Run(args []string) error {
	c, err := New(RELAY)
	if err != nil {
		return fmt.Errorf("New: %w", err)
	}

	go KeepBootnodes(c, args)

	relay := fmt.Sprintf("%s%s?persist=1", RELAY, SUBPATH)

	return wtf.Serve(relay, c)
}
