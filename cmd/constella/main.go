package main

import (
	"log"
	"os"

	"github.com/btwiuse/constella"
)

func main() {
	if err := constella.Run(os.Args[1:]); err != nil {
		log.Fatalln(err)
	}
}
