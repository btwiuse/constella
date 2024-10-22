package constella

import (
	"github.com/webteleport/wtf"
)

func Run(args []string) error {
	host := New(RELAY)
	return wtf.Serve(RELAY, host)
}
