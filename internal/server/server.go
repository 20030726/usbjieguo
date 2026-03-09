package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"usbjieguo/internal/discovery"
	"usbjieguo/internal/storage"
)

const version = "0.1.0"

// ReceivedFile is published to the Events channel each time a file arrives.
type ReceivedFile struct {
	Original string // name from the sender
	Saved    string // name actually written to disk
	Size     int    // bytes
}

// Server wraps the HTTP server and the UDP discovery listener.
type Server struct {
	port    int
	name    string
	storage *storage.Storage
	Events  chan ReceivedFile // non-nil when TUI mode; nil means log-only
}

// New creates a Server with the given configuration.
// Pass a non-nil events channel to receive ReceivedFile notifications.
func New(port int, name string, store *storage.Storage) *Server {
	return &Server{port: port, name: name, storage: store}
}

// NewWithEvents is like New but wires up an event channel for TUI consumers.
func NewWithEvents(port int, name string, store *storage.Storage, events chan ReceivedFile) *Server {
	return &Server{port: port, name: name, storage: store, Events: events}
}

// CheckPort tries to bind both the TCP HTTP port and the UDP discovery port.
// Returns an error if either is already in use.
func CheckPort(httpPort int) error {
	// Check TCP (HTTP server)
	ln, err := net.Listen("tcp4", fmt.Sprintf(":%d", httpPort))
	if err != nil {
		return fmt.Errorf("TCP port %d: %w", httpPort, err)
	}
	ln.Close()

	// Check UDP (discovery listener)
	udp, err := net.ListenPacket("udp4", fmt.Sprintf(":%d", discovery.UDPPort))
	if err != nil {
		return fmt.Errorf("UDP port %d (discovery): %w", discovery.UDPPort, err)
	}
	udp.Close()

	return nil
}

// SetSaveDir changes the storage directory at runtime.
func (s *Server) SetSaveDir(dir string) error {
	return s.storage.SetDir(dir)
}

// Start launches the UDP discovery listener and the HTTP server.
// It blocks until the HTTP server exits.
func (s *Server) Start() error {
	go s.listenDiscovery()

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", s.handlePing)
	mux.HandleFunc("/info", s.handleInfo)
	mux.HandleFunc("/upload", s.handleUpload)

	addr := fmt.Sprintf(":%d", s.port)
	log.Printf("HTTP server listening on %s", addr)
	return http.ListenAndServe(addr, mux)
}

// ── Handlers ─────────────────────────────────────────────────────────────────

// GET /ping
func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, "ok")
}

type infoResponse struct {
	Name    string `json:"name"`
	Port    int    `json:"port"`
	Version string `json:"version"`
}

// GET /info
func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(infoResponse{
		Name:    s.name,
		Port:    s.port,
		Version: version,
	})
}

type uploadResponse struct {
	Status   string `json:"status"`
	Filename string `json:"filename"`
}

// POST /upload
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Limit memory used for in-memory multipart buffering to 32 MB.
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "bad request: missing 'file' field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "internal error: cannot read file", http.StatusInternalServerError)
		return
	}

	savedName, err := s.storage.Save(header.Filename, data)
	if err != nil {
		http.Error(w, "internal error: cannot save file", http.StatusInternalServerError)
		return
	}

	// If the sender flagged this as a directory zip, extract it.
	if r.Header.Get("X-Is-Dir") == "true" {
		folderName, err := s.storage.SaveAndUnzip(header.Filename, data)
		if err != nil {
			http.Error(w, "internal error: cannot unzip directory", http.StatusInternalServerError)
			return
		}
		// Remove the intermediate .zip file, we only want the extracted folder.
		_ = os.Remove(filepath.Join(s.storage.Dir(), savedName))
		savedName = folderName + "/"
	}

	log.Printf("received: %s (%d bytes) -> saved as %s", header.Filename, len(data), savedName)

	if s.Events != nil {
		select {
		case s.Events <- ReceivedFile{Original: header.Filename, Saved: savedName, Size: len(data)}:
		default:
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(uploadResponse{
		Status:   "ok",
		Filename: savedName,
	})
}

// ── UDP Discovery Listener ────────────────────────────────────────────────────

// listenDiscovery listens for UDP discovery broadcasts and replies with
// "name:httpPort" so the sender can build the full address.
func (s *Server) listenDiscovery() {
	udpAddr := fmt.Sprintf(":%d", discovery.UDPPort)
	conn, err := net.ListenPacket("udp4", udpAddr)
	if err != nil {
		log.Printf("discovery listener error: %v", err)
		return
	}
	defer conn.Close()

	log.Printf("discovery listening on UDP %s", udpAddr)
	buf := make([]byte, 256)

	for {
		n, remote, err := conn.ReadFrom(buf)
		if err != nil {
			continue
		}

		msg := strings.TrimSpace(string(buf[:n]))
		if msg == discovery.DiscoverMsg {
			reply := fmt.Sprintf("%s:%d", s.name, s.port)
			if _, err := conn.WriteTo([]byte(reply), remote); err != nil {
				log.Printf("discovery reply error: %v", err)
			}
		}
	}
}
