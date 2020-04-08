package main

import (
	"fmt"
	"swjkja"
)

func main() {
	err := swjkja.StartDeamon(29060)
	fmt.Print(err)
}
