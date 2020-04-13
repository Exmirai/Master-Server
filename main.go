package main

import (
	"flag"

	"github.com/exmirai/Master-Server/internal/swjkja"
)

var port int

func main() {
	flag.IntVar(&port, "port", 29060, "UDP Packets listener port")
	flag.Parse()
	swjkja.StartDeamon(29060)
}
