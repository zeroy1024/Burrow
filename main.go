package main

import (
	"log"

	"github.com/zeroy1024/Burrow/internal/desktop"
)

func main() {
	if err := desktop.Run(); err != nil {
		log.Fatal(err)
	}
}
