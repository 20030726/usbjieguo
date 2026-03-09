# usbjieguo

Lightweight LAN file transfer CLI tool. No USB, no cloud — send files across the same network with a single command.

---

## Installation

### Requirements

- Go 1.22+
- Same LAN (`discover` relies on UDP broadcast — devices must be on the same subnet)
- Firewall must allow TCP port `8787` (HTTP) and UDP port `9797` (discovery)

### macOS / Linux

```bash
# Enter the project directory
cd usbjieguo

# Build and install to /usr/local/bin (requires make)
make install

# Or build to the current directory only
make build
# → produces ./usbjieguo

# Manual install
sudo mv usbjieguo /usr/local/bin/
```

### Windows

```powershell
# Enter the project directory
cd usbjieguo

# Build
go build -o usbjieguo.exe .

# Copy usbjieguo.exe to a directory already in your system PATH
# e.g. C:\Users\<you>\bin\
```

### Cross-platform build (macOS / Linux)

```bash
make build-all
# Produces binaries for all platforms under dist/:
#   usbjieguo-darwin-amd64
#   usbjieguo-darwin-arm64
#   usbjieguo-linux-amd64
#   usbjieguo-linux-arm64
#   usbjieguo-windows-amd64.exe
```

---

## Quick Start

### 0. TUI interactive mode (recommended)

The easiest way to use usbjieguo. Launch it and navigate everything through the menu:

```bash
usbjieguo tui
```

Custom options:

```bash
usbjieguo tui --port 9000 --dir /tmp/files --name my-pc
```

| Flag | Description | Default |
|------|-------------|---------|
| `--port` | Listening port (used when Serve is selected) | `8787` |
| `--dir` | Save directory (used when Serve is selected) | `./recv` |
| `--name` | Device display name | hostname |

Press `Ctrl-C` to quit.

#### TUI Keyboard Shortcuts

**Main menu / list pages**

| Key | Action |
|-----|--------|
| `↑` / `↓` | Move selection |
| `→` / `Enter` | Select / enter |
| `Ctrl-C` | Quit |

**Serve page**

| Key | Action |
|-----|--------|
| Type any text | Fuzzy-filter the directory listing |
| `↑` / `↓` / `Ctrl-P` / `Ctrl-N` | Move cursor |
| `→` / `Enter` | Enter directory (also sets it as the save directory) |
| `←` / `Backspace` | Go up one level (deletes a character when search bar is non-empty) |
| `s` | Set current directory as save directory (only when search bar is empty) |
| `r` | Refresh directory (only when search bar is empty) |
| `Ctrl-U` | Clear search bar |
| `q` / `Q` | Return to main menu (only when search bar is empty, otherwise types the character) |
| `Esc` | Return to main menu |

**Send — scan page**

| Key | Action |
|-----|--------|
| `↑` / `↓` | Move cursor |
| `→` / `Enter` | Select receiver |
| `r` | Re-scan |
| `←` / `Esc` / `q` | Return to main menu |

**Send — file picker page**

| Key | Action |
|-----|--------|
| Type any text | Fuzzy-filter files |
| `↑` / `↓` / `Ctrl-P` / `Ctrl-N` | Move cursor |
| `Enter` | Enter directory / send file |
| `→` | Enter directory |
| `←` / `Backspace` | Go up one level (deletes a character when search bar is non-empty) |
| `Ctrl-U` | Clear search bar |
| `Esc` / `Ctrl-Q` | Return to scan page |

**Sending page**

| Key | Action |
|-----|--------|
| `Enter` / `Esc` / `←` / `→` | Return to file picker after transfer completes |
| `q` / `Q` | Go back to peer list to pick a different target |

---

### 1. Receiver: start the server

On the **machine that will receive files**:

```bash
usbjieguo serve
```

Defaults:
- HTTP listening on port **8787**
- Files saved to **`./recv/`**
- Device name uses the hostname

Custom options:

```bash
usbjieguo serve --port 9000 --dir /tmp/files --name my-pc
```

| Flag | Description | Default |
|------|-------------|---------|
| `--port` | Listening port | `8787` |
| `--dir` | Save directory | `./recv` |
| `--name` | Device display name | hostname |

---

### 2. Sender: scan the LAN (optional)

If you don't know the receiver's IP, scan first:

```bash
usbjieguo discover
```

Example output:

```
scanning LAN for receivers (3s)...
KC-MacBook           192.168.0.103:8787
lab-pi               192.168.0.212:8787
```

> The receiver must be running `usbjieguo serve` to appear in the list.

---

### 3. Sender: send a file

```bash
usbjieguo send ./report.pdf --to 192.168.0.212:8787
```

Success output:

```
file sent successfully
saved as: report.pdf
```

If the target already has a file with the same name, the server renames it automatically (`report(1).pdf`, `report(2).pdf`, …).

---

## Full example

```
# [Machine A — receiver]
$ usbjieguo serve --dir ~/downloads
serving on port 8787, saving to ~/downloads (device: MacBook-A)

# [Machine B — sender]
$ usbjieguo discover
scanning LAN for receivers (3s)...
MacBook-A            192.168.0.50:8787

$ usbjieguo send ./data.zip --to 192.168.0.50:8787
file sent successfully
saved as: data.zip
```

---

## Troubleshooting

| Error | Likely cause | Fix |
|-------|-------------|-----|
| `target not reachable` | Receiver not running or wrong IP/port | Confirm receiver is running and the IP is correct |
| `no receivers found` | `discover` found nobody | Confirm both devices are on the same subnet and the receiver is running |
| `file not found` | Wrong file path | Confirm the path exists |
| `--to flag is required` | Missing `--to` flag | Add `--to host:port` |

---

## HTTP API (advanced)

You can call the receiver directly with curl or any HTTP client:

```bash
# Health check
curl http://192.168.0.212:8787/ping

# Device info
curl http://192.168.0.212:8787/info

# Upload a file
curl -F "file=@./test.txt" http://192.168.0.212:8787/upload
```

---

## Security notice

> **⚠️ v1 transfers are unencrypted.**
>
> All files are sent over plain HTTP. Anyone on the same subnet can intercept the traffic with a packet sniffer (e.g. Wireshark).
> **Do not transfer files containing passwords, keys, or other sensitive data.**
>
> Recommended for use on trusted private networks only (home LAN, lab intranet). Encryption support is planned for a future release.

---
