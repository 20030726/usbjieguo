package cmd

import (
	"flag"
	"fmt"
	"os"

	"usbjieguo/internal/client"
)

// RunSend parses send flags and uploads a file to the target.
func RunSend(args []string) {
	fs := flag.NewFlagSet("send", flag.ExitOnError)
	to := fs.String("to", "", "target address in host:port format")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: usbjieguo send <file> --to <host:port>")
		os.Exit(1)
	}
	if *to == "" {
		fmt.Fprintln(os.Stderr, "send failed: --to flag is required")
		os.Exit(1)
	}

	filePath := fs.Arg(0)

	c := client.New(*to)
	saved, err := c.Send(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "send failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("file sent successfully")
	if saved != "" {
		fmt.Println("saved as:", saved)
	}
}
