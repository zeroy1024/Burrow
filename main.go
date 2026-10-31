package main

import (
	"log"

	"sshdesk/internal/desktop"
)

func main() {
	if err := desktop.Run(); err != nil {
		log.Fatal(err)
	}
}
