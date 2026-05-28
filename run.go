package constella

import (
	"context"
	"log"
	"net/http"

	ma "github.com/multiformats/go-multiaddr"
	"github.com/webteleport/utils"
)

func ConnectBootnodes(c *Constella, addrs []string) {
	for _, addr := range addrs {
		maddr, err := ma.NewMultiaddr(addr)
		if err != nil {
			log.Println("invalid bootnode addr:", addr, err)
			continue
		}
		peerInfo, err := AddrInfo(maddr)
		if err != nil {
			log.Println("failed to resolve bootnode:", addr, err)
			continue
		}
		err = c.Host.Connect(context.Background(), *peerInfo)
		if err != nil {
			log.Println("failed to connect bootnode:", addr, err)
			continue
		}
	}
}

func Run(args []string) error {
	port := utils.EnvPort(":8080")
	c := New(RELAY)
	log.Println("listening on", port)

	go ConnectBootnodes(c, args)

	return http.ListenAndServe(port, c)
}
