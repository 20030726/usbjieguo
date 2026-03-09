package tui

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	figure "github.com/common-nighthawk/go-figure"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"usbjieguo/internal/client"
	"usbjieguo/internal/discovery"
	"usbjieguo/internal/server"
	"usbjieguo/internal/storage"
)

// ── Pages ─────────────────────────────────────────────────────────────────────

type page int

const (
	pageMenu page = iota
	pageServeConfig // configure port before starting server
	pageServe
	pageScanPeers
	pageFilePicker // browse filesystem to pick a file
	pageSending
)

// ── Banner ───────────────────────────────────────────────────────────────────

var (
	appBannerLarge = strings.TrimRight(figure.NewFigure("usbjieguo", "small", true).String(), "\n")
	appBannerSmall = strings.TrimRight(figure.NewFigure("usbjieguo", "mini", true).String(), "\n")
)

// chooseBanner returns the best-fitting banner for the given terminal dimensions.
// reservedH is the number of rows needed for everything below the banner.
func chooseBanner(w, h, reservedH int) string {
	switch {
	case bannerFits(appBannerLarge, w, h, reservedH):
		return appBannerLarge
	case bannerFits(appBannerSmall, w, h, reservedH):
		return appBannerSmall
	default:
		if w > 0 {
			return "usbjieguo"
		}
		return ""
	}
}

// bannerFits reports whether banner fits inside termW×termH,
// given that the rest of the menu needs reservedH additional rows.
func bannerFits(banner string, termW, termH, reservedH int) bool {
	if termW <= 0 || termH <= 0 {
		return false
	}
	lines := strings.Split(banner, "\n")
	for _, l := range lines {
		if len(l) > termW {
			return false
		}
	}
	return len(lines)+reservedH <= termH
}

// ── Styles ────────────────────────────────────────────────────────────────────

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("86")).
			MarginBottom(1)

	statusOK  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	statusErr = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	dimStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	boldStyle = lipgloss.NewStyle().Bold(true)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)
)

// ── Menu items ────────────────────────────────────────────────────────────────

type menuItem struct{ label, desc string }

func (m menuItem) Title() string       { return m.label }
func (m menuItem) Description() string { return m.desc }
func (m menuItem) FilterValue() string { return m.label }



// ── Messages ──────────────────────────────────────────────────────────────────

type scanDoneMsg struct{ peers []discovery.Peer }
type scanErrMsg struct{ err error }
type autoRescanMsg struct{}
type scanCountdownTickMsg struct{} // per-second tick to update countdown display
type fileReceivedMsg server.ReceivedFile
type sendDoneMsg struct{ err error }
type servePollTickMsg struct{} // fired when pollServeEvents times out with no new file
type serverStartedMsg struct {
	err error
	srv *server.Server // non-nil on success
}

// ── Model ─────────────────────────────────────────────────────────────────────

type Model struct {
	page page
	w, h int

	// Menu
	menuList list.Model

	// Serve config
	servePortInput textinput.Model
	serveConfigErr string

	// Serve
	servePort    int
	serveName    string
	serveDir     string
	serveEvents  chan server.ReceivedFile
	serveLog     []string
	newestFile   string          // last received file (Original name)
	serveActive  bool            // true once server goroutine is running
	activeServer *server.Server  // reference for runtime changes (e.g. save dir)
	serveBrowser browserModel    // folder browser on the serve page

	// Send – peer scan
	peerSpinner  spinner.Model
	peerList     list.Model
	scanning     bool
	scanErr      string
	scanRescanAt time.Time // when the next auto-rescan will fire

	// Send – peer selection
	selectedPeer discovery.Peer

	// Send – file/dir browser
	browser    browserModel
	browserErr string

	// Send – progress
	progress  progress.Model
	sendDone  bool
	sendErr   string
	sendFile  string

}

// New creates the root TUI model.
func New(servePort int, serveName, serveDir string) Model {
	// Menu list
	menuItems := []list.Item{
		menuItem{"Serve", fmt.Sprintf("Start receiver (default port %d)", servePort)},
		menuItem{"Send", "Scan LAN and send a file"},
	}
	mList := list.New(menuItems, list.NewDefaultDelegate(), 40, 10)
	mList.Title = "usbjieguo"
	mList.SetShowStatusBar(false)
	mList.SetFilteringEnabled(false)
	mList.SetShowHelp(false)
	mList.Styles.Title = titleStyle
	mList.KeyMap.Quit = key.NewBinding() // 移除 list 內建的 q/Q quit，統一用 ctrl+c 退出

	// Peer list (starts empty)
	pList := list.New(nil, list.NewDefaultDelegate(), 40, 10)
	pList.Title = "LAN Receivers"
	pList.SetShowStatusBar(false)
	pList.SetFilteringEnabled(false)
	pList.Styles.Title = titleStyle
	pList.KeyMap.Quit = key.NewBinding() // 移除 list 內建的 q/Q quit，統一用 ctrl+c 退出

	// Spinner
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	// Port config input
	pi := textinput.New()
	pi.Placeholder = strconv.Itoa(servePort)
	pi.SetValue(strconv.Itoa(servePort))
	pi.Width = 10

	// File/dir browser – prefer Desktop or Downloads over bare home directory.
	startDir := preferredStartDir()

	// Progress bar
	pg := progress.New(progress.WithDefaultGradient())

	pickerBrowser := newBrowser(startDir)
	pickerBrowser.alwaysSearch = true
	pickerBrowser.input.Focus()

	serveBrowserInit := newBrowser(serveDir)
	serveBrowserInit.alwaysSearch = true
	serveBrowserInit.input.Focus()

	return Model{
		page:           pageMenu,
		menuList:       mList,
		peerSpinner:    sp,
		peerList:       pList,
		servePortInput: pi,
		browser:        pickerBrowser,
		serveBrowser:   serveBrowserInit,
		progress:       pg,
		servePort:      servePort,
		serveName:      serveName,
		serveDir:       serveDir,
		serveEvents:    make(chan server.ReceivedFile, 32),
	}
}

// ── Init ──────────────────────────────────────────────────────────────────────

func (m Model) Init() tea.Cmd {
	return nil
}

// ── Update ────────────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		const menuReservedH = 6 // hints + borders
		bannerText := chooseBanner(msg.Width, msg.Height, menuReservedH+3)
		bannerLines := 0
		if bannerText != "" {
			bannerLines = strings.Count(bannerText, "\n") + 3 // text lines + surrounding newlines
		}
		menuH := msg.Height - bannerLines - menuReservedH
		if menuH < 6 {
			menuH = 6
		}
		m.menuList.SetSize(msg.Width-4, menuH)
		m.peerList.SetSize(msg.Width-4, msg.Height-10)
		m.progress.Width = msg.Width - 8
		m.browser.setSize(msg.Width-4, msg.Height-4)
		m.serveBrowser.setSize(msg.Width-4, msg.Height-14)
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

	case fileReceivedMsg:
		line := fmt.Sprintf("%s  ←  %s  (%d B)",
			time.Now().Format("15:04:05"), msg.Saved, msg.Size)
		m.serveLog = append(m.serveLog, line)
		m.newestFile = msg.Original
		_ = m.serveBrowser.cd(m.serveBrowser.currentDir) // auto-refresh on receive
		return m, pollServeEvents(m.serveEvents)

	case serverStartedMsg:
		if msg.err != nil {
			m.serveConfigErr = msg.err.Error()
			m.page = pageServeConfig
			m.serveActive = false
		} else {
			m.serveActive = true
			m.activeServer = msg.srv
			if abs, err := filepath.Abs(m.serveDir); err == nil {
				m.serveDir = abs
			}
			m.serveBrowser = newBrowser(m.serveDir)
			m.serveBrowser.alwaysSearch = true
			m.serveBrowser.input.Focus()
			m.serveBrowser.setSize(m.w-4, m.h-14)
			m.page = pageServe
		}
		return m, nil

	case scanDoneMsg:
		m.scanning = false
		items := make([]list.Item, len(msg.peers))
		for i, p := range msg.peers {
			items[i] = peerItem{p}
		}
		m.peerList.SetItems(items)
		if len(msg.peers) == 0 {
			m.scanErr = "No receivers found on LAN"
		}
		m.scanRescanAt = time.Now().Add(5 * time.Second)
		return m, tea.Batch(scheduleRescan(), scanCountdownTick())

	case autoRescanMsg:
		// Fire only if still on the scan page and not already scanning.
		if m.page == pageScanPeers && !m.scanning {
			m.scanning = true
			m.scanErr = ""
			return m, tea.Batch(m.peerSpinner.Tick, scanPeers())
		}
		return m, nil

	case scanErrMsg:
		m.scanning = false
		m.scanErr = msg.err.Error()
		m.scanRescanAt = time.Now().Add(5 * time.Second)
		// Retry automatically after 5 seconds on error.
		return m, tea.Batch(
			tea.Tick(5*time.Second, func(time.Time) tea.Msg { return autoRescanMsg{} }),
			scanCountdownTick(),
		)

	case sendDoneMsg:
		m.sendDone = true
		if msg.err != nil {
			m.sendErr = msg.err.Error()
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.peerSpinner, cmd = m.peerSpinner.Update(msg)
		return m, cmd

	case progress.FrameMsg:
		pg, cmd := m.progress.Update(msg)
		m.progress = pg.(progress.Model)
		return m, cmd

	case servePollTickMsg:
		// Keep the polling loop alive even when no file has arrived yet.
		if m.serveActive {
			return m, pollServeEvents(m.serveEvents)
		}
		return m, nil

	case scanCountdownTickMsg:
		// Keep ticking once per second while on scan page and not scanning.
		if m.page == pageScanPeers && !m.scanning {
			return m, scanCountdownTick()
		}
		return m, nil
	}

	switch m.page {
	case pageMenu:
		return m.updateMenu(msg)
	case pageServeConfig:
		return m.updateServeConfig(msg)
	case pageServe:
		return m.updateServe(msg)
	case pageScanPeers:
		return m.updateScan(msg)
	case pageFilePicker:
		return m.updateFilePicker(msg)
	case pageSending:
		return m.updateSending(msg)
	}
	return m, nil
}

// updateMenu handles input on the main menu page.
func (m Model) updateMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q", "Q":
			// q/Q 不退出，使用 ctrl+c 退出
			return m, nil
		case "enter", "right":
			item, ok := m.menuList.SelectedItem().(menuItem)
			if !ok {
				break
			}
			switch item.label {
			case "Serve":
				if m.serveActive {
					// Server already running — just show the status page
					m.page = pageServe
					return m, nil
				}
				// Go to port-config page first
				m.page = pageServeConfig
				m.serveConfigErr = ""
				m.servePortInput.Focus()
				return m, textinput.Blink
			case "Send":
				m.page = pageScanPeers
				m.scanning = true
				m.scanErr = ""
				m.peerList.SetItems(nil)
				return m, tea.Batch(m.peerSpinner.Tick, scanPeers())
			}
		}
	}
	m.menuList, cmd = m.menuList.Update(msg)
	return m, cmd
}

// updateServeConfig handles port-configuration before starting the server.
func (m Model) updateServeConfig(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.page = pageMenu
			return m, nil
		case "enter":
			raw := strings.TrimSpace(m.servePortInput.Value())
			port, err := strconv.Atoi(raw)
			if err != nil || port < 1 || port > 65535 {
				m.serveConfigErr = fmt.Sprintf("invalid port: %q (must be 1–65535)", raw)
				break
			}
			// Check port availability before starting server
			if err := server.CheckPort(port); err != nil {
				m.serveConfigErr = fmt.Sprintf("port %d is already in use: %v", port, err)
				break
			}
			m.servePort = port
			m.serveConfigErr = ""
			m.serveLog = nil
			return m, tea.Batch(
				startServer(port, m.serveName, m.serveDir, m.serveEvents),
				pollServeEvents(m.serveEvents),
			)
		}
	}
	m.servePortInput, cmd = m.servePortInput.Update(msg)
	return m, cmd
}

// updateServe handles input on the serve page.
func (m Model) updateServe(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, isKey := msg.(tea.KeyMsg)

	if isKey {
		switch key.String() {
		case "esc":
			m.page = pageMenu
			return m, nil

		case "q", "Q":
			// 只在搜尋欄空白時才返回選單，避免打字時意外跳出
			if m.serveBrowser.input.Value() == "" {
				m.page = pageMenu
				return m, nil
			}
			var cmdQ tea.Cmd
			m.serveBrowser.input, cmdQ = m.serveBrowser.input.Update(msg)
			m.serveBrowser.applyFilter()
			return m, cmdQ

		case "ctrl+u":
			m.serveBrowser.input.SetValue("")
			m.serveBrowser.applyFilter()
			return m, nil

		case "up", "ctrl+p":
			var cmd tea.Cmd
			m.serveBrowser, cmd = m.serveBrowser.Update(tea.KeyMsg{Type: tea.KeyUp})
			m.serveBrowser.updatePreview()
			return m, cmd

		case "down", "ctrl+n":
			var cmd tea.Cmd
			m.serveBrowser, cmd = m.serveBrowser.Update(tea.KeyMsg{Type: tea.KeyDown})
			m.serveBrowser.updatePreview()
			return m, cmd

		case "enter", "right":
			if entry, ok := m.serveBrowser.highlighted(); ok && entry.isDir {
				_ = m.serveBrowser.cd(entry.path)
				if m.activeServer != nil {
					if err := m.activeServer.SetSaveDir(entry.path); err == nil {
						m.serveDir = entry.path
					}
				}
				return m, nil
			}

		case "s":
			// 只在搜尋欄空白時才設定儲存目錄，避免打字時觸發
			if m.serveBrowser.input.Value() == "" {
				if m.activeServer != nil {
					if err := m.activeServer.SetSaveDir(m.serveBrowser.currentDir); err == nil {
						m.serveDir = m.serveBrowser.currentDir
					}
				}
				return m, nil
			}
			var cmdS tea.Cmd
			m.serveBrowser.input, cmdS = m.serveBrowser.input.Update(msg)
			m.serveBrowser.applyFilter()
			return m, cmdS

		case "r":
			// 只在搜尋欄空白時才重新整理，避免打字時觸發
			if m.serveBrowser.input.Value() == "" {
				_ = m.serveBrowser.cd(m.serveBrowser.currentDir)
				return m, nil
			}
			var cmdR tea.Cmd
			m.serveBrowser.input, cmdR = m.serveBrowser.input.Update(msg)
			m.serveBrowser.applyFilter()
			return m, cmdR

		case "left":
			// ← 鍵：輸入欄空白時往上一層，否則忽略
			if m.serveBrowser.input.Value() == "" {
				m.serveBrowser.up()
			}
			return m, nil

		case "backspace":
			// backspace：永遠只刪文字，不觸發上一層
			var cmd tea.Cmd
			m.serveBrowser.input, cmd = m.serveBrowser.input.Update(msg)
			m.serveBrowser.applyFilter()
			return m, cmd

		default:
			if key.Type == tea.KeyRunes {
				var cmd tea.Cmd
				m.serveBrowser.input, cmd = m.serveBrowser.input.Update(msg)
				m.serveBrowser.applyFilter()
				return m, cmd
			}
		}
		var cmd tea.Cmd
		m.serveBrowser, cmd = m.serveBrowser.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	m.serveBrowser, cmd = m.serveBrowser.Update(msg)
	return m, cmd
}

// updateScan handles input on the peer-scan page.
func (m Model) updateScan(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "Q", "esc", "left":
			m.page = pageMenu
			m.scanning = false
			return m, nil
		case "enter", "right":
			if m.scanning || len(m.peerList.Items()) == 0 {
				break
			}
			sel, ok := m.peerList.SelectedItem().(peerItem)
			if !ok {
				break
			}
			m.selectedPeer = sel.Peer
			m.page = pageFilePicker
			m.browserErr = ""
			return m, nil
		case "r":
			// Re-scan
			m.scanning = true
			m.scanErr = ""
			m.peerList.SetItems(nil)
			return m, tea.Batch(m.peerSpinner.Tick, scanPeers())
		}
	}
	m.peerList, cmd = m.peerList.Update(msg)
	return m, cmd
}

// updateFilePicker handles input on the file/dir browser page.
func (m Model) updateFilePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			// esc 返回掃描頁面（不會與打字衝突）
			m.page = pageScanPeers
			if !m.scanning {
				m.scanning = true
				m.scanErr = ""
				m.peerList.SetItems(nil)
				return m, tea.Batch(m.peerSpinner.Tick, scanPeers())
			}
			return m, nil

		case "ctrl+q":
			if !m.scanning {
				m.scanning = true
				m.scanErr = ""
				m.peerList.SetItems(nil)
				return m, tea.Batch(m.peerSpinner.Tick, scanPeers())
			}
			return m, nil

		case "ctrl+u":
			m.browser.input.SetValue("")
			m.browser.applyFilter()
			return m, nil

		case "up", "ctrl+p":
			var cmd tea.Cmd
			m.browser, cmd = m.browser.Update(tea.KeyMsg{Type: tea.KeyUp})
			m.browser.updatePreview()
			return m, cmd

		case "down", "ctrl+n":
			var cmd tea.Cmd
			m.browser, cmd = m.browser.Update(tea.KeyMsg{Type: tea.KeyDown})
			m.browser.updatePreview()
			return m, cmd

		case "enter":
			entry, hasEntry := m.browser.highlighted()
			if !hasEntry {
				return m, nil
			}
			if entry.isDir {
				if err := m.browser.cd(entry.path); err == nil {
					m.browserErr = ""
				} else {
					m.browserErr = err.Error()
				}
				return m, nil
			}
			return m, m.startSend(entry.path)

		case "right":
			if entry, ok2 := m.browser.highlighted(); ok2 && entry.isDir {
				if err := m.browser.cd(entry.path); err == nil {
					m.browserErr = ""
				} else {
					m.browserErr = err.Error()
				}
				return m, nil
			}

		case "left", "backspace":
			if m.browser.input.Value() == "" {
				m.browser.up()
				m.browserErr = ""
				return m, nil
			}
			// Non-empty input: let the text input handle deletion.
			var cmd tea.Cmd
			m.browser.input, cmd = m.browser.input.Update(msg)
			m.browser.applyFilter()
			return m, cmd

		default:
			if key.Type == tea.KeyRunes {
				var cmd tea.Cmd
				m.browser.input, cmd = m.browser.input.Update(msg)
				m.browser.applyFilter()
				return m, cmd
			}
		}
	}

	var cmd tea.Cmd
	m.browser, cmd = m.browser.Update(msg)
	return m, cmd
}

// startSend transitions to pageSending and returns the upload command.
func (m *Model) startSend(path string) tea.Cmd {
	m.browserErr = ""
	m.sendDone = false
	m.sendErr = ""
	m.sendFile = path
	m.page = pageSending
	target := fmt.Sprintf("%s:%d", m.selectedPeer.IP, m.selectedPeer.Port)
	return tea.Batch(sendFileCmd(target, path), m.peerSpinner.Tick)
}

// updateSending handles input on the sending/progress page.
func (m Model) updateSending(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if !m.sendDone {
			break
		}
		switch msg.String() {
		case "esc", "enter", "left", "right":
			// Same peer — go back to file picker.
			m.page = pageFilePicker
			m.browserErr = ""
			if !m.scanning {
				m.scanning = true
				m.scanErr = ""
				return m, tea.Batch(m.peerSpinner.Tick, scanPeers())
			}
		case "q", "Q":
			// Different peer — go back to peer list.
			m.page = pageScanPeers
			m.scanning = true
			m.scanErr = ""
			m.peerList.SetItems(nil)
			return m, tea.Batch(m.peerSpinner.Tick, scanPeers())
		}
	}
	return m, nil
}

// ── View ──────────────────────────────────────────────────────────────────────

func (m Model) View() string {
	switch m.page {
	case pageMenu:
		return m.viewMenu()
	case pageServeConfig:
		return m.viewServeConfig()
	case pageServe:
		return m.viewServe()
	case pageScanPeers:
		return m.viewScan()
	case pageFilePicker:
		return m.viewFilePicker()
	case pageSending:
		return m.viewSending()
	}
	return ""
}

func (m Model) viewMenu() string {
	bannerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("86")).
		Bold(true)

	const reservedH = 6
	var banner string
	if b := chooseBanner(m.w, m.h, reservedH+3); b != "" {
		banner = bannerStyle.Render(b) + "\n"
	}

	var srvStatus string
	if m.serveActive {
		srvStatus = "\n" + statusOK.Render(fmt.Sprintf("  ● Server running on :%d  (save → %s)", m.servePort, m.serveDir))
	}

	return "\n" + banner + m.menuList.View() + srvStatus + "\n" + dimStyle.Render("  ↑/↓ navigate  →/enter select  ctrl+c exit")
}

func (m Model) viewServeConfig() string {
	header := titleStyle.Render("Start Receiver")
	hint := dimStyle.Render("  enter start  esc back")
	errLine := ""
	if m.serveConfigErr != "" {
		errLine = "\n" + statusErr.Render("  ✗ "+m.serveConfigErr)
	}
	return "\n" + header + "\n\n  " + boldStyle.Render("Port:") + "\n  " +
		m.servePortInput.View() + errLine + "\n\n" + hint
}

func (m Model) viewServe() string {
	saveLine := dimStyle.Render("  save → ") + boldStyle.Render(m.serveDir)
	header := titleStyle.Render(fmt.Sprintf("Serving on :%d  (device: %s)", m.servePort, m.serveName))

	// Recent received files — compact log panel (last 4 entries)
	var logLines string
	if len(m.serveLog) == 0 {
		logLines = dimStyle.Render("  waiting for files…")
	} else {
		const maxLogLines = 4
		start := 0
		if len(m.serveLog) > maxLogLines {
			start = len(m.serveLog) - maxLogLines
		}
		newStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true)
		lines := make([]string, len(m.serveLog[start:]))
		for i, l := range m.serveLog[start:] {
			if start+i == len(m.serveLog)-1 {
				lines[i] = newStyle.Render(l + "  ← new")
			} else {
				lines[i] = l
			}
		}
		logLines = strings.Join(lines, "\n")
	}
	logBox := boxStyle.Render(logLines)

	hint := dimStyle.Render(
		"  ↑/↓ ctrl+p/n  →/enter cd  s save  r refresh  " +
			"←/⌫ up  ctrl+u clear  esc/q back")

	return "\n" + header + "\n" + saveLine + "\n" + logBox + "\n" + m.serveBrowser.View() + "\n" + hint
}

func (m Model) viewScan() string {
	header := titleStyle.Render("Send a File")

	if m.scanning {
		hint := dimStyle.Render("  ←/esc back")
		return "\n" + header + "\n\n  " + m.peerSpinner.View() + " Scanning LAN…\n\n" + hint
	}

	secs := int(time.Until(m.scanRescanAt).Seconds())
	if secs < 0 {
		secs = 0
	}
	countdown := dimStyle.Render(fmt.Sprintf("  (next scan in %ds)", secs))
	hint := dimStyle.Render("  →/enter select  ←/esc back  r rescan") + "  " + countdown
	if m.scanErr != "" {
		return "\n" + header + "\n\n" + statusErr.Render("  ✗ "+m.scanErr) + "\n" + countdown + "\n\n" + hint
	}
	return "\n" + m.peerList.View() + "\n\n" + hint
}

func (m Model) viewFilePicker() string {
	header := titleStyle.Render(fmt.Sprintf("Browse → %s  (%s:%d)",
		m.selectedPeer.Name, m.selectedPeer.IP, m.selectedPeer.Port))

	hint := dimStyle.Render(
		"  ↑/↓ ctrl+p/n navigate  enter open/send  → dir only  " +
			"←/backspace up  ctrl+u clear  esc/ctrl+q back")
	errLine := ""
	if m.browserErr != "" {
		errLine = "\n" + statusErr.Render("  ✗ "+m.browserErr)
	}
	return "\n" + header + "\n" + m.browser.View() + errLine + "\n" + hint
}

func (m Model) viewSending() string {
	header := titleStyle.Render(fmt.Sprintf("Sending → %s  (%s:%d)",
		m.selectedPeer.Name, m.selectedPeer.IP, m.selectedPeer.Port))

	if m.sendDone {
		hint := dimStyle.Render("  enter/←/→/esc 再傳一個  q 換目標機器")
		if m.sendErr != "" {
			return "\n" + header + "\n\n" + statusErr.Render("  ✗ "+m.sendErr) + "\n\n" + hint
		}
		return "\n" + header + "\n\n" + statusOK.Render("  ✓ "+m.sendFile+" 傳送成功！") + "\n\n" + hint
	}

	hint := dimStyle.Render("  please wait…")
	stats := fmt.Sprintf("  %s  uploading…", m.sendFile)
	return "\n" + header + "\n\n  " + m.peerSpinner.View() + " " + stats + "\n\n" + hint
}

// ── Peer list item ────────────────────────────────────────────────────────────

type peerItem struct{ discovery.Peer }

func (p peerItem) Title() string       { return p.Name }
func (p peerItem) Description() string { return fmt.Sprintf("%s:%d", p.IP, p.Port) }
func (p peerItem) FilterValue() string { return p.Name }

// ── Commands ──────────────────────────────────────────────────────────────────

// startServer starts the serve + discovery goroutines in the background.
// It sends a serverStartedMsg so the TUI can react to port-conflict errors.
func startServer(port int, name, dir string, events chan server.ReceivedFile) tea.Cmd {
	return func() tea.Msg {
		// Double-check port before handing off to goroutine.
		if err := server.CheckPort(port); err != nil {
			return serverStartedMsg{err: err}
		}
		store := storage.New(dir)
		_ = store.Init()
		srv := server.NewWithEvents(port, name, store, events)
		go func() { _ = srv.Start() }()
		return serverStartedMsg{srv: srv}
	}
}

// pollServeEvents waits up to 500 ms for a ReceivedFile event.
// Using a timeout prevents the goroutine from blocking forever if the server
// stops unexpectedly; servePollTickMsg will re-queue the poll.
func pollServeEvents(events chan server.ReceivedFile) tea.Cmd {
	return func() tea.Msg {
		select {
		case rf := <-events:
			return fileReceivedMsg(rf)
		case <-time.After(500 * time.Millisecond):
			return servePollTickMsg{}
		}
	}
}

// scanPeers runs the UDP discovery scan on a goroutine.
func scanPeers() tea.Cmd {
	return func() tea.Msg {
		peers, err := discovery.Scan()
		if err != nil {
			return scanErrMsg{err}
		}
		return scanDoneMsg{peers}
	}
}

// scheduleRescan waits 5 s then fires autoRescanMsg.
func scheduleRescan() tea.Cmd {
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg {
		return autoRescanMsg{}
	})
}

// scanCountdownTick fires scanCountdownTickMsg every second to update the countdown display.
func scanCountdownTick() tea.Cmd {
	return tea.Tick(1*time.Second, func(time.Time) tea.Msg {
		return scanCountdownTickMsg{}
	})
}

// sendFileCmd uploads the file (or directory) and returns sendDoneMsg when finished.
func sendFileCmd(target, path string) tea.Cmd {
	return func() tea.Msg {
		c := client.New(target)
		info, err := os.Stat(path)
		if err != nil {
			return sendDoneMsg{fmt.Errorf("stat: %w", err)}
		}
		if info.IsDir() {
			err = c.SendDir(path)
		} else {
			_, err = c.Send(path)
		}
		return sendDoneMsg{err}
	}
}

// preferredStartDir returns the best directory to open the file picker.
// Tries ~/Desktop then ~/Downloads before falling back to the home directory.
func preferredStartDir() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return "."
	}
	for _, sub := range []string{"Desktop", "Downloads"} {
		p := filepath.Join(home, sub)
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p
		}
	}
	return home
}

// Run starts the Bubble Tea program.
func Run(servePort int, serveName, serveDir string) error {
	// Silence the standard logger so server log.Printf calls don't corrupt the TUI.
	log.SetOutput(io.Discard)

	m := New(servePort, serveName, serveDir)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
