// This build-only proxy packs Windows executables before MyGo signs and packages them.
package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	args := os.Args[1:]
	if err := runCommand(os.Getenv("BURROW_BUILD_GO"), args...); err != nil {
		return err
	}
	windowsBuild := len(args) > 0 && args[0] == "build" && os.Getenv("GOOS") == "windows"
	if !windowsBuild {
		return nil
	}
	outputIndex := slices.Index(args, "-o")
	if outputIndex < 0 || outputIndex+1 == len(args) {
		return errors.New("windows build requires an explicit -o output path")
	}
	output := args[outputIndex+1]
	upx := os.Getenv("BURROW_BUILD_UPX")
	if err := runCommand(
		upx,
		"--lzma",
		"-9",
		"--compress-icons=0",
		"--strip-relocs=0",
		output,
	); err != nil {
		return err
	}
	return runCommand(upx, "-t", output)
}

func runCommand(name string, args ...string) error {
	command := exec.Command(name, args...)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(name), err)
	}
	return nil
}
