package cmd

import (
	"flag"
	"fmt"
	"os"

	"usbjieguo/internal/server"
	"usbjieguo/internal/storage"
)

// RunServe parses serve flags and starts the HTTP + UDP server.
func RunServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	port := fs.Int("port", 8787, "listening port")
	dir := fs.String("dir", "./recv", "save directory")
	name := fs.String("name", hostname(), "device name")
	fs.Parse(args)

	store := storage.New(*dir)
	if err := store.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to init save directory: %v\n", err)
		os.Exit(1)
	}

	srv := server.New(*port, *name, store)
	fmt.Printf("serving on port %d, saving to %s (device: %s)\n", *port, *dir, *name)

	if err := srv.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}
