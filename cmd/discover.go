package cmd

import (
	"flag"
	"fmt"

	"usbjieguo/internal/discovery"
)

// RunDiscover broadcasts a discovery packet and prints found receivers.
func RunDiscover(args []string) {
	fs := flag.NewFlagSet("discover", flag.ExitOnError)
	fs.Parse(args)

	fmt.Println("scanning LAN for receivers (3s)...")

	peers, err := discovery.Scan()
	if err != nil {
		fmt.Printf("discover error: %v\n", err)
		return
	}

	if len(peers) == 0 {
		fmt.Println("no receivers found")
		return
	}

	for _, p := range peers {
		fmt.Printf("%-20s %s:%d\n", p.Name, p.IP, p.Port)
	}
}
