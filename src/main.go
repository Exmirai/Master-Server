package main

import (
	"flag"
	"fmt"
	"swjkja"
)

var port int

func main() {
	flag.IntVar(&port, "port", 29060, "UDP Packets listener port")
	flag.Parse()
	err := swjkja.StartDeamon(29060)
	fmt.Print(err)
}
