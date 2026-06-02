package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "fetch-vendor":
		FetchVendor()
	case "build":
		Build()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("VIT Build Scripts")
	fmt.Println()
	fmt.Println("Usage: go run ./scripts/build/*.go <command> [args...]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  fetch-vendor              Download vendored binaries (fzf, git)")
	fmt.Println("  build                     Build VIT locally")
}
