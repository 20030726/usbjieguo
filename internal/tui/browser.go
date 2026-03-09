package tui

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

// fsEntry is one file-system row shown inside browserModel.
type fsEntry struct {
	path  string
	name  string
	isDir bool
	size  int64
}

var dirEntryStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Bold(true)
var matchStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true) // highlighted chars

func (e fsEntry) Title() string {
	if e.isDir {
		return dirEntryStyle.Render("▶ " + e.name + "/")
	}
	return "  " + e.name
}

// titleWithMatches renders the name with fuzzy-match character positions highlighted.
func (e fsEntry) titleWithMatches(positions []int) string {
	posSet := make(map[int]bool, len(positions))
	for _, p := range positions {
		posSet[p] = true
	}
	runes := []rune(e.name)
	var out string
	for i, r := range runes {
		s := string(r)
		if posSet[i] {
			out += matchStyle.Render(s)
		} else {
			out += s
		}
	}
	if e.isDir {
		return dirEntryStyle.Render("▶ ") + out + dirEntryStyle.Render("/")
	}
	return "  " + out
}

func (e fsEntry) Description() string {
	if e.isDir {
		return "directory"
	}
	return humanBytes(e.size)
}

func (e fsEntry) FilterValue() string { return e.name }

func humanBytes(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(1024), 0
	for n := b / 1024; n >= 1024; n /= 1024 {
		div *= 1024
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// browserModel is a navigable file-system list.
// When alwaysSearch=true (telescope mode): search bar is always visible,
// input is always focused, and a preview pane appears on the right.
// When alwaysSearch=false (serve page directory browser): legacy Shift+F toggle.
type browserModel struct {
	list       list.Model
	currentDir string

	// fuzzy search state
	allItems []fsEntry     // full unfiltered directory contents
	searching bool         // toggle mode only (alwaysSearch=false)
	input     textinput.Model
	matches   []fuzzy.Match

	// telescope mode extras
	alwaysSearch bool
	preview      string
	w, h         int
}

func newBrowser(startDir string) browserModel {
	d := list.NewDefaultDelegate()
	d.ShowDescription = false // single-line items like Telescope
	l := list.New(nil, d, 60, 20)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)
	l.Styles.Title = titleStyle
	l.KeyMap.Quit = key.NewBinding() // 移除 list 內建的 q/Q quit

	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "fuzzy search…"
	ti.CharLimit = 64

	b := browserModel{list: l, input: ti}
	_ = b.cd(startDir)
	return b
}

// setSize stores dimensions and resizes the internal list.
// In telescope mode the list takes the left pane only.
func (b *browserModel) setSize(w, h int) {
	b.w = w
	b.h = h
	if b.alwaysSearch {
		leftW, _ := b.paneWidths()
		listH := h - 4 // 3 lines for search bar border + 1 gap
		if listH < 3 {
			listH = 3
		}
		b.list.SetSize(leftW, listH)
	} else {
		b.list.SetSize(w, h)
	}
}

// paneWidths returns (leftW, rightW). rightW==0 when terminal is too narrow.
func (b browserModel) paneWidths() (left, right int) {
	if b.w <= 60 {
		return b.w, 0
	}
	left = b.w * 40 / 100
	right = b.w - left - 1 // 1 for the divider column
	return
}

// cd switches to dir and reloads the entry list.
func (b *browserModel) cd(dir string) error {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	// Directories first, then files — each group alphabetical.
	sort.Slice(entries, func(i, j int) bool {
		di, dj := entries[i].IsDir(), entries[j].IsDir()
		if di != dj {
			return di
		}
		return entries[i].Name() < entries[j].Name()
	})

	items := make([]fsEntry, 0, len(entries))
	for _, e := range entries {
		var size int64
		if !e.IsDir() {
			if info, err2 := e.Info(); err2 == nil {
				size = info.Size()
			}
		}
		items = append(items, fsEntry{
			path:  filepath.Join(dir, e.Name()),
			name:  e.Name(),
			isDir: e.IsDir(),
			size:  size,
		})
	}

	b.currentDir = dir
	b.list.Title = dir
	b.allItems = items
	// Always reset search / filter when changing directory.
	b.searching = false
	b.input.SetValue("")
	b.applyFilter() // applyFilter calls updatePreview when alwaysSearch=true
	return nil
}

// applyFilter rebuilds the visible list using the current query.
func (b *browserModel) applyFilter() {
	query := b.input.Value()
	if query == "" {
		b.matches = nil
		listItems := make([]list.Item, len(b.allItems))
		for i, e := range b.allItems {
			listItems[i] = e
		}
		b.list.SetItems(listItems)
	} else {
		names := make([]string, len(b.allItems))
		for i, e := range b.allItems {
			names[i] = e.name
		}
		b.matches = fuzzy.Find(query, names)
		listItems := make([]list.Item, len(b.matches))
		for i, m := range b.matches {
			listItems[i] = b.allItems[m.Index]
		}
		b.list.SetItems(listItems)
	}
	if b.alwaysSearch {
		b.updatePreview()
	}
}

// up navigates to the parent directory.
func (b *browserModel) up() {
	parent := filepath.Dir(b.currentDir)
	if parent != b.currentDir {
		_ = b.cd(parent)
	}
}

// highlighted returns the currently selected list entry.
func (b browserModel) highlighted() (fsEntry, bool) {
	item := b.list.SelectedItem()
	if item == nil {
		return fsEntry{}, false
	}
	e, ok := item.(fsEntry)
	return e, ok
}

// ── Preview ───────────────────────────────────────────────────────────────────

func (b *browserModel) updatePreview() {
	entry, ok := b.highlighted()
	if !ok {
		b.preview = ""
		return
	}
	maxLines := b.h - 6
	if maxLines < 5 {
		maxLines = 5
	}
	b.preview = computePreview(entry, maxLines)
}

func computePreview(e fsEntry, maxLines int) string {
	if e.isDir {
		entries, err := os.ReadDir(e.path)
		if err != nil {
			return "  (cannot read directory)"
		}
		lines := []string{fmt.Sprintf("  %d items", len(entries))}
		for i, en := range entries {
			if len(lines) >= maxLines {
				lines = append(lines, fmt.Sprintf("  … (%d more)", len(entries)-i))
				break
			}
			if en.IsDir() {
				lines = append(lines, dirEntryStyle.Render("  ▶ "+en.Name()+"/"))
			} else {
				lines = append(lines, "    "+en.Name())
			}
		}
		return strings.Join(lines, "\n")
	}
	// File: try to read as text.
	f, err := os.Open(e.path)
	if err != nil {
		return "  " + humanBytes(e.size) + "\n  (cannot read)"
	}
	defer f.Close()
	buf := make([]byte, 8192)
	n, _ := f.Read(buf)
	content := buf[:n]
	if bytes.ContainsRune(content, 0) {
		return fmt.Sprintf("  Binary file\n  Size: %s", humanBytes(e.size))
	}
	lines := strings.Split(string(content), "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	result := make([]string, len(lines))
	for i, l := range lines {
		result[i] = "  " + l
	}
	return strings.Join(result, "\n")
}

var (
	searchPromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	searchBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("220")).
				Padding(0, 1)
	previewDivStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("237"))
	previewHeaderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Bold(true)
)

// buildListView returns the left-pane list with fuzzy highlights applied.
// It uses a clone so b.list internals (selection cursor) are never corrupted.
func (b browserModel) buildListView(w, h int) string {
	clone := b.list
	clone.SetSize(w, h)
	clone.Title = "Results"
	if b.input.Value() != "" && len(b.matches) > 0 {
		cloneItems := make([]list.Item, len(b.matches))
		for i, m := range b.matches {
			e := b.allItems[m.Index]
			cloneItems[i] = renderedEntry{
				fsEntry:       e,
				renderedTitle: e.titleWithMatches(m.MatchedIndexes),
			}
		}
		clone.SetItems(cloneItems)
	}
	return clone.View()
}

// View renders the browser.
// Telescope mode (alwaysSearch=true):  left list | divider | right preview
//                                      ─────────────── search bar ────────
// Legacy mode: list + optional search bar overlay.
func (b browserModel) View() string {
	if !b.alwaysSearch {
		// ── Legacy mode (serve-page directory browser) ──────────────────────
		listView := b.list.View()
		if b.searching {
			if b.input.Value() != "" && len(b.matches) > 0 {
				cloneItems := make([]list.Item, len(b.matches))
				for i, m := range b.matches {
					e := b.allItems[m.Index]
					cloneItems[i] = renderedEntry{fsEntry: e, renderedTitle: e.titleWithMatches(m.MatchedIndexes)}
				}
				clone := b.list
				clone.SetItems(cloneItems)
				return clone.View() + "\n" + b.searchBarView()
			}
			return listView + "\n" + b.searchBarView()
		}
		return listView
	}

	// ── Telescope mode ───────────────────────────────────────────────────────
	leftW, rightW := b.paneWidths()
	listH := b.h - 4
	if listH < 3 {
		listH = 3
	}

	leftView := b.buildListView(leftW, listH)

	if rightW < 20 {
		return leftView + "\n" + b.searchBarView()
	}

	// Build right preview pane.
	previewBody := lipgloss.NewStyle().
		Width(rightW - 1).
		Height(listH - 1).
		Render(b.preview)
	rightPane := previewHeaderStyle.Render("  Preview") + "\n" + previewBody

	// Vertical divider (same height as list including title row).
	divLines := make([]string, listH+1)
	for i := range divLines {
		divLines[i] = "│"
	}
	divider := previewDivStyle.Render(strings.Join(divLines, "\n"))

	main := lipgloss.JoinHorizontal(lipgloss.Top, leftView, divider, rightPane)
	return main + "\n" + b.searchBarView()
}

// searchBarView renders the Telescope-style prompt line.
func (b browserModel) searchBarView() string {
	total := len(b.allItems)
	var countStr string
	if b.input.Value() != "" {
		countStr = fmt.Sprintf(" %d / %d", len(b.matches), total)
	} else {
		countStr = fmt.Sprintf(" %d", total)
	}
	counter := dimStyle.Render(countStr)
	prompt := searchPromptStyle.Render("> ")
	inner := prompt + b.input.View() + counter
	barWidth := b.w - 4
	if barWidth < 10 {
		barWidth = 10
	}
	return searchBorderStyle.Width(barWidth).Render(inner)
}

// renderedEntry wraps fsEntry with a pre-rendered title (for highlight display).
type renderedEntry struct {
	fsEntry
	renderedTitle string
}

func (r renderedEntry) Title() string       { return r.renderedTitle }
func (r renderedEntry) Description() string { return r.fsEntry.Description() }
func (r renderedEntry) FilterValue() string { return r.fsEntry.FilterValue() }

func (b browserModel) Update(msg tea.Msg) (browserModel, tea.Cmd) {
	var cmd tea.Cmd
	b.list, cmd = b.list.Update(msg)
	return b, cmd
}
