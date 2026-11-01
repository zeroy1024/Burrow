package main

import (
	"log"

	_ "github.com/egoist/mygo/locales/zh-Hans"

	"sshdesk/internal/desktop"
)

func main() {
	if err := desktop.Run(); err != nil {
		log.Fatal(err)
	}
}
