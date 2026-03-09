package cmd

import (
	"flag"
	"fmt"
	"os"

	"usbjieguo/internal/tui"
)

// RunTUI starts the interactive Bubble Tea TUI.
func RunTUI(args []string) {
	fs := flag.NewFlagSet("tui", flag.ExitOnError)
	port := fs.Int("port", 8787, "listening port (used when you choose Serve)")
	dir := fs.String("dir", "./recv", "save directory (used when you choose Serve)")
	name := fs.String("name", hostname(), "device name")
	fs.Parse(args)

	if err := tui.Run(*port, *name, *dir); err != nil {
		fmt.Fprintf(os.Stderr, "tui error: %v\n", err)
		os.Exit(1)
	}
}
