package main

import (
	"os"
	"vit/internal/cli"
)

func main() {
	exitCode := cli.Main()
	os.Exit(exitCode)
}
