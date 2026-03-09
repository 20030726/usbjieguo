package discovery

import (
	"fmt"
	"net"
	"strings"
	"time"
)

const (
	// UDPPort is the port the server listens on for discovery packets.
	UDPPort = 9797
	// DiscoverMsg is the payload sent by the discover command.
	DiscoverMsg = "usbjieguo-discover"
	// ScanTimeout is how long Scan waits for responses.
	ScanTimeout = 3 * time.Second
)

// Peer represents a discovered receiver on the LAN.
type Peer struct {
	Name string
	IP   string
	Port int
}

// Scan broadcasts a discovery packet and collects Peer responses for ScanTimeout.
func Scan() ([]Peer, error) {
	// Bind to any local UDP port; replies will arrive here.
	localAddr, err := net.ResolveUDPAddr("udp4", ":0")
	if err != nil {
		return nil, fmt.Errorf("resolve local addr: %w", err)
	}
	conn, err := net.ListenUDP("udp4", localAddr)
	if err != nil {
		return nil, fmt.Errorf("cannot open UDP socket: %w", err)
	}
	defer conn.Close()

	// Broadcast the discovery message BEFORE setting the read deadline, so
	// deadline does not affect the outgoing writes.
	broadcastAll(conn)

	// Set read deadline so we stop collecting after ScanTimeout.
	if err := conn.SetReadDeadline(time.Now().Add(ScanTimeout)); err != nil {
		return nil, fmt.Errorf("set deadline: %w", err)
	}

	// Collect responses until deadline.
	var peers []Peer
	buf := make([]byte, 256)

	for {
		n, remote, err := conn.ReadFrom(buf)
		if err != nil {
			// Deadline exceeded or other read error – stop collecting.
			break
		}

		resp := strings.TrimSpace(string(buf[:n]))

		// Response format from server: "name:httpPort"
		// Use LastIndex so device names containing colons are handled.
		lastColon := strings.LastIndex(resp, ":")
		if lastColon < 0 {
			continue
		}

		name := resp[:lastColon]
		portStr := resp[lastColon+1:]

		var port int
		if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
			continue
		}

		remoteIP := remote.(*net.UDPAddr).IP.String()
		peers = append(peers, Peer{Name: name, IP: remoteIP, Port: port})
	}

	return peers, nil
}

// broadcastAll sends the discovery message to 255.255.255.255 and also to
// every active interface's directed subnet broadcast address.
//
// Sending only to 255.255.255.255 (the limited broadcast) is not always
// delivered on Linux when multiple interfaces are up or a VPN is running.
// Sending to the directed broadcast (e.g. 192.168.1.255) is more reliable
// and works on macOS, Linux, and Windows.
func broadcastAll(conn *net.UDPConn) {
	msg := []byte(DiscoverMsg)

	// 1. Limited broadcast – works well on Windows and simple setups.
	conn.WriteTo(msg, &net.UDPAddr{IP: net.IPv4bcast, Port: UDPPort}) //nolint:errcheck

	// 2. Directed broadcasts per interface – more reliable on Linux/macOS.
	ifaces, err := net.Interfaces()
	if err != nil {
		return // best-effort; limited broadcast above may still work
	}
	seen := make(map[string]bool)
	for _, iface := range ifaces {
		if iface.Flags&net.FlagBroadcast == 0 ||
			iface.Flags&net.FlagUp == 0 ||
			iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipnet.IP.To4()
			if ip4 == nil {
				continue
			}
			bcast := make(net.IP, 4)
			for i := 0; i < 4; i++ {
				bcast[i] = ip4[i] | ^ipnet.Mask[i]
			}
			bcastStr := bcast.String()
			if seen[bcastStr] {
				continue // skip duplicate broadcast addresses
			}
			seen[bcastStr] = true
			conn.WriteTo(msg, &net.UDPAddr{IP: bcast, Port: UDPPort}) //nolint:errcheck
		}
	}
}
