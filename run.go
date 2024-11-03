package constella

import (
	"log"
	"net/http"

	"github.com/webteleport/utils"
)

func Run(args []string) error {
	port := utils.EnvPort(":8080")
	constella := New(RELAY)
	log.Println("listening on", port)
	return http.ListenAndServe(port, constella)
}
