package main

import (
	"log"

	"github.com/jonmorehouse/safe/safe"
)

func main() {
	if err := safe.NewCLI(); err != nil {
		log.Fatal(err)
	}
}
