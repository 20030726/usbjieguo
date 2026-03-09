# usbjieguo

輕量級 LAN 檔案傳輸 CLI 工具。無需 USB、無需雲端，同一網路下一行指令即可傳檔。

---

## 安裝

### 需求

- Go 1.22+
- 同一 LAN（`discover` 依賴 UDP broadcast，需同網段）
- 防火牆需允許 TCP port `8787`（HTTP）與 UDP port `9797`（discovery）

### macOS / Linux

```bash
# 進入專案目錄
cd usbjieguo

# 編譯並安裝到 /usr/local/bin（需要 make）
make install

# 或只編譯到當前目錄
make build
# → 產生 ./usbjieguo

# 手動安裝
sudo mv usbjieguo /usr/local/bin/
```

### Windows

```powershell
# 進入專案目錄
cd usbjieguo

# 編譯
go build -o usbjieguo.exe .

# 把 usbjieguo.exe 複製到已在系統 PATH 的目錄
# 例如：C:\Users\<你的名字>\bin\
```

### 跨平台一次編譯（macOS / Linux）

```bash
make build-all
# 在 dist/ 下產生所有平台的 binary：
#   usbjieguo-darwin-amd64
#   usbjieguo-darwin-arm64
#   usbjieguo-linux-amd64
#   usbjieguo-linux-arm64
#   usbjieguo-windows-amd64.exe
```

---

## 快速開始

### 0. TUI 互動介面（推薦）

最簡單的使用方式，啟動後透過選單操作所有功能：

```bash
usbjieguo tui
```

自訂選項：

```bash
usbjieguo tui --port 9000 --dir /tmp/files --name my-pc
```

| 旗標 | 說明 | 預設值 |
|------|------|--------|
| `--port` | 監聽 port（選擇 Serve 時使用） | `8787` |
| `--dir` | 儲存目錄（選擇 Serve 時使用） | `./recv` |
| `--name` | 裝置顯示名稱 | 主機名稱 |

按 `Ctrl-C` 退出。

#### TUI 鍵盤快捷鍵

**主選單 / 列表頁面**

| 按鍵 | 動作 |
|------|------|
| `↑` / `↓` | 移動選項 |
| `→` / `Enter` | 選擇 / 進入 |
| `Ctrl-C` | 退出程式 |

**Serve 接收頁（Telescope 檔案瀏覽）**

| 按鍵 | 動作 |
|------|------|
| 輸入任意文字 | Fuzzy 搜尋過濾 |
| `↑` / `↓` / `Ctrl-P` / `Ctrl-N` | 移動游標 |
| `→` / `Enter` | 進入目錄（同時設定儲存目錄） |
| `←` / `Backspace` | 上一層目錄（搜尋欄有字時刪除字元） |
| `s` | 設定目前瀏覽目錄為儲存目錄（搜尋欄空白時有效） |
| `r` | 重新整理目錄（搜尋欄空白時有效） |
| `Ctrl-U` | 清除搜尋欄 |
| `q` / `Q` | 返回主選單（搜尋欄空白時有效，否則輸入字元） |
| `Esc` | 返回主選單 |

**Send 掃描頁**

| 按鍵 | 動作 |
|------|------|
| `↑` / `↓` | 移動游標 |
| `→` / `Enter` | 選擇接收端 |
| `r` | 重新掃描 |
| `←` / `Esc` / `q` | 返回主選單 |

**Send 檔案選擇頁（Telescope 模式）**

| 按鍵 | 動作 |
|------|------|
| 輸入任意文字 | Fuzzy 搜尋過濾 |
| `↑` / `↓` / `Ctrl-P` / `Ctrl-N` | 移動游標 |
| `Enter` | 進入目錄 / 送出檔案 |
| `→` | 進入目錄 |
| `←` / `Backspace` | 上一層（搜尋欄有字時刪除字元） |
| `Ctrl-U` | 清除搜尋欄 |
| `Esc` / `Ctrl-Q` | 返回掃描頁 |
| `Ctrl-F` | 呼叫 Neovim Telescope 選檔（需在 Neovim `:terminal` 內） |

**傳送中頁面**

| 按鍵 | 動作 |
|------|------|
| `Enter` / `Esc` / `←` / `→` | 傳送完成後返回檔案選擇頁 |

---

### 1. 接收端：啟動伺服器

在**要接收檔案的機器**上執行：

```bash
usbjieguo serve
```

預設行為：
- HTTP 監聽 port **8787**
- 收到的檔案存到 **`./recv/`**
- 裝置名稱使用主機名稱

自訂選項：

```bash
usbjieguo serve --port 9000 --dir /tmp/files --name my-pc
```

| 旗標 | 說明 | 預設值 |
|------|------|--------|
| `--port` | 監聽 port | `8787` |
| `--dir` | 儲存目錄 | `./recv` |
| `--name` | 裝置顯示名稱 | 主機名稱 |

---

### 2. 傳送端：掃描區網（可選）

不確定接收端 IP 時，先掃描：

```bash
usbjieguo discover
```

輸出範例：

```
scanning LAN for receivers (3s)...
KC-MacBook           192.168.0.103:8787
lab-pi               192.168.0.212:8787
```

> 接收端必須正在執行 `usbjieguo serve` 才會出現在清單中。

---

### 3. 傳送端：送出檔案

```bash
usbjieguo send ./report.pdf --to 192.168.0.212:8787
```

成功輸出：

```
file sent successfully
saved as: report.pdf
```

若目標已有同名檔案，伺服器自動重新命名（`report(1).pdf`、`report(2).pdf` …）。

---

## 完整範例流程

```
# [機器 A - 接收端]
$ usbjieguo serve --dir ~/downloads
serving on port 8787, saving to ~/downloads (device: MacBook-A)

# [機器 B - 傳送端]
$ usbjieguo discover
scanning LAN for receivers (3s)...
MacBook-A            192.168.0.50:8787

$ usbjieguo send ./data.zip --to 192.168.0.50:8787
file sent successfully
saved as: data.zip
```

---

## 錯誤排查

| 錯誤訊息 | 可能原因 | 解法 |
|----------|----------|------|
| `target not reachable` | 接收端未啟動或 IP/port 錯誤 | 確認接收端正在執行，IP 無誤 |
| `no receivers found` | discover 掃不到任何人 | 確認同一網段，且接收端有啟動 |
| `file not found` | 傳送的檔案路徑錯誤 | 確認路徑存在 |
| `--to flag is required` | 忘記加 `--to` 旗標 | 補上 `--to host:port` |

---

## HTTP API（進階）

可用 curl 或其他工具直接呼叫：

```bash
# 健康檢查
curl http://192.168.0.212:8787/ping

# 取得裝置資訊
curl http://192.168.0.212:8787/info

# 上傳檔案
curl -F "file=@./test.txt" http://192.168.0.212:8787/upload
```

---

## 安全性注意事項

> **⚠️ v1 傳輸不加密。**
>
> 所有檔案透過明文 HTTP 傳輸，同網段的人可以用封包嗅探工具（如 Wireshark）攔截內容。
> **請勿傳輸含有密碼、金鑰或個人敏感資料的檔案。**
>
> 僅建議在受信任的私人網路（家用 LAN、lab 內網）使用。加密功能預計在未來版本加入。

---

## Neovim 整合

可在 Neovim 裡用浮動視窗開啟 TUI，或用 Telescope 啟動 serve 和瀏覽已接收的檔案。

### 需要的 Plugin

- [lazy.nvim](https://github.com/folke/lazy.nvim)（plugin manager）
- [telescope.nvim](https://github.com/nvim-telescope/telescope.nvim)
- [telescope-file-browser.nvim](https://github.com/nvim-telescope/telescope-file-browser.nvim)

### 設定方式

將以下內容加入你的 `init.lua`（Windows：`%LOCALAPPDATA%\nvim\init.lua`，macOS/Linux：`~/.config/nvim/init.lua`）：

```lua
-- Bootstrap lazy.nvim
local lazypath = vim.fn.stdpath("data") .. "/lazy/lazy.nvim"
if not vim.loop.fs_stat(lazypath) then
  vim.fn.system({
    "git", "clone", "--filter=blob:none",
    "https://github.com/folke/lazy.nvim.git",
    "--branch=stable", lazypath,
  })
end
vim.opt.rtp:prepend(lazypath)

require("lazy").setup({
  { "nvim-telescope/telescope.nvim", dependencies = { "nvim-lua/plenary.nvim" } },
  { "nvim-telescope/telescope-file-browser.nvim",
    dependencies = { "nvim-telescope/telescope.nvim", "nvim-lua/plenary.nvim" } },
})

-- TUI 浮動視窗
local function open_usbjieguo()
  local width  = math.floor(vim.o.columns * 0.85)
  local height = math.floor(vim.o.lines   * 0.85)
  local buf = vim.api.nvim_create_buf(false, true)
  vim.api.nvim_open_win(buf, true, {
    relative = "editor", width = width, height = height,
    row = math.floor((vim.o.lines - height) / 2),
    col = math.floor((vim.o.columns - width) / 2),
    style = "minimal", border = "rounded",
    title = " usbjieguo ", title_pos = "center",
  })
  local cmd = vim.fn.has("win32") == 1
    and { "cmd", "/c", "usbjieguo", "tui" } or { "usbjieguo", "tui" }
  vim.fn.termopen(cmd, {
    on_exit = function()
      if vim.api.nvim_buf_is_valid(buf) then
        vim.api.nvim_buf_delete(buf, { force = true })
      end
    end,
  })
  vim.keymap.set("t", "<C-q>", "<C-c>", { buffer = buf, noremap = true })
  vim.cmd("startinsert")
end

-- Serve（背景執行）
local serve_job_id = nil
local serve_dir    = vim.fn.expand("~") .. "/recv"

local function serve_start(dir)
  if serve_job_id then return end
  serve_dir = dir or serve_dir
  local cmd = vim.fn.has("win32") == 1
    and { "cmd", "/c", "usbjieguo", "serve", "--dir", serve_dir }
    or  { "usbjieguo", "serve", "--dir", serve_dir }
  serve_job_id = vim.fn.jobstart(cmd, {
    on_exit = function() serve_job_id = nil
      vim.notify("usbjieguo serve 已停止", vim.log.levels.INFO)
    end,
  })
  vim.notify("usbjieguo serve 啟動 → " .. serve_dir, vim.log.levels.INFO)
end

local function usbjieguo_serve()
  require("telescope").load_extension("file_browser")
  require("telescope").extensions.file_browser.file_browser({
    prompt_title = "選擇接收目錄 (Enter 確認)",
    dir_only = true,
    attach_mappings = function(prompt_bufnr, map)
      local actions = require("telescope.actions")
      local state   = require("telescope.actions.state")
      local function confirm()
        local entry = state.get_selected_entry()
        actions.close(prompt_bufnr)
        serve_start(entry and (entry.path or entry[1]) or serve_dir)
      end
      map("i", "<CR>", confirm) ; map("n", "<CR>", confirm)
      return true
    end,
  })
end

local function usbjieguo_browse()
  require("telescope.builtin").find_files({
    prompt_title = "已接收的檔案 ← " .. serve_dir,
    cwd = serve_dir, hidden = true, no_ignore = true,
  })
end

vim.g.mapleader = " "
vim.keymap.set("n", "<leader>u",  open_usbjieguo,                           { desc = "usbjieguo: TUI" })
vim.keymap.set("n", "<leader>us", usbjieguo_serve,                           { desc = "usbjieguo: start serve" })
vim.keymap.set("n", "<leader>uS", function() vim.fn.jobstop(serve_job_id) end, { desc = "usbjieguo: stop serve" })
vim.keymap.set("n", "<leader>ur", usbjieguo_browse,                          { desc = "usbjieguo: browse received" })
```

### 快捷鍵

| 按鍵 | 功能 |
|------|------|
| `Space + u` | 開啟 usbjieguo TUI（浮動視窗） |
| `Space + us` | 用 Telescope 選目錄後啟動 serve（背景執行） |
| `Space + uS` | 停止 serve |
| `Space + ur` | 用 Telescope 瀏覽已接收的檔案 |
| `Ctrl-Q` | 退出 TUI（在浮動視窗內） |
| `Ctrl-\ Ctrl-N` | 跳回 Neovim normal mode（不關 TUI） |

> `usbjieguo` binary 需在 PATH 中。Windows 請確認安裝目錄已加入系統 PATH。
