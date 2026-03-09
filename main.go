package main

import (
	"fmt"
	"os"

	"usbjieguo/cmd"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		cmd.RunServe(os.Args[2:])
	case "send":
		cmd.RunSend(os.Args[2:])
	case "discover":
		cmd.RunDiscover(os.Args[2:])
	case "tui":
		cmd.RunTUI(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: usbjieguo <command> [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  tui        Interactive TUI (recommended)")
	fmt.Println("             --port int    listening port (default 8787)")
	fmt.Println("             --dir  string save directory (default ./recv)")
	fmt.Println("             --name string device name   (default hostname)")
	fmt.Println()
	fmt.Println("  serve      Start file receiving server")
	fmt.Println("             --port int    listening port (default 8787)")
	fmt.Println("             --dir  string save directory (default ./recv)")
	fmt.Println("             --name string device name   (default hostname)")
	fmt.Println()
	fmt.Println("  send       Send a file to another device")
	fmt.Println("             <file> --to host:port")
	fmt.Println()
	fmt.Println("  discover   Scan LAN for available receivers")
}
