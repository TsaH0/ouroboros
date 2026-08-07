# Ouroboros

<div align="center">

```
  ___    ___  ___  ___      _
 / _ \ / __\/ _ \/ __\__ _(_)__
| | | / /  | | | / /  / // / _ \
| |_|/ /___| |_|/ /___/ _  /  __/
 \___/\____/\___/\____/_//_/\___|
```

**A lightweight, AI-powered web security workbench for the terminal.**

</div>

---

## Why Ouroboros?

Ouroboros is a terminal-first HTTP security workbench — an intercepting proxy, traffic inspector, request repeater, and AI-assisted analyzer in a single Go binary. No Electron, no Docker, no 500 MB of RAM just to inspect a request. It runs in your terminal, uses less than 30 MB of memory, and gets out of your way.

### Key benefits

- **AI-assisted vulnerability analysis** — Pipe captured traffic to any LLM (OpenAI, Gemini, NVIDIA NIM, or local Ollama) for instant security analysis. Get severity-ranked findings, exploitation context, and remediation suggestions without leaving the terminal.
- **Multi-pane workspace** — Split your screen into History, Repeater, Scope, and Recon panes. All panes run independently — captured traffic keeps flowing while you analyze, replay, and pivot.
- **Vim/tmux keybindings** — Navigate panes with `Ctrl+h/j/k/l`, split with `Ctrl+w s/v`, open views with number keys. Zero learning curve if you live in the terminal.
- **Integrated recon** — Subfinder, gau, waybackurls, whatweb, and searchsploit are orchestrated automatically. Enter a domain, get a full recon report with hosts, endpoints, technologies, and known CVEs — then send it to AI for prioritization.
- **Scope management** — Define exactly which hosts are in scope. Out-of-scope traffic is filtered from view. Repeater blocks replay to out-of-scope targets. AI analysis only considers in-scope flows. Safe testing by design.
- **Request repeater** — Capture, edit, and replay any HTTP request. Modify headers, body, method, URL. See the response inline. Scope enforcement prevents accidental out-of-scope requests.
- **SQLite persistence** — All flows, scope rules, recon results, and AI analyses are stored in a local SQLite database with WAL mode. Restart the app and your data is still there.
- **Project save/load** — Vim-style `:w`/`:e` commands save and load scope configurations as named project files. Switch between engagements instantly.
- **Minimal footprint** — Pure Go, single binary, no runtime dependencies. ~30 MB RAM with a full proxy, MITM, and TUI running. Compare that to 500 MB+ for browser-based alternatives.

---

## Quick Start

Requires Go 1.25+.

```sh
git clone https://github.com/TsaH0/ouroboros.git
cd ouroboros
make run
```

The proxy listens on `:8080`. Set your browser/system proxy to `127.0.0.1:8080` and browse. Captured traffic appears in the History pane.

### HTTPS interception

```sh
# Print the CA certificate for browser installation
go run ./cmd/ouroboros --install-ca
```

Import the printed certificate into your browser's trust store to enable HTTPS MITM.

---

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│  TUI (Bubble Tea v2)                                          │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐         │
│  │ History  │ │ Repeater │ │  Scope   │ │  Recon   │         │
│  └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘         │
│       │            │            │            │                │
│       ▼            ▼            ▼            ▼                │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │  Workspace Manager (panes, splits, focus, key routing) │  │
│  └───────────────────────────┬─────────────────────────────┘  │
└──────────────────────────────┼───────────────────────────────┘
                               │
┌──────────────────────────────┼───────────────────────────────┐
│  Domain Layer                │                                │
│  ┌──────────────────────────▼──────────────────────────────┐ │
│  │  Services (Scope, Intercept, Repeater, LLM Analyzer)    │ │
│  └──────────────────────────┬──────────────────────────────┘ │
│  ┌──────────────────────────▼──────────────────────────────┐ │
│  │  Store                                                   │ │
│  │  ┌──────────────┐    ┌──────────────────────────────┐    │ │
│  │  │ InMemory    │    │ SQLite (WAL, migrations)     │    │ │
│  │  └──────────────┘    └──────────────────────────────┘    │ │
│  └──────────────────────────────────────────────────────────┘ │
│  ┌──────────────────────────────────────────────────────────┐ │
│  │  Proxy Engine                                            │ │
│  │  ┌──────────────┐    ┌──────────────────────────────┐    │ │
│  │  │ HTTP Handler │    │ MITM (CONNECT, TLS)          │    │ │
│  │  └──────────────┘    └──────────────────────────────┘    │ │
│  └──────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────┘
```

---

## Keybindings

### Global (any pane)

| Key | Action |
|-----|--------|
| `:` | Open command bar (vim-style: `:w`, `:e`, `:ls`, `:q`) |
| `0` | Spawn HTTP History pane |
| `4` | Open Scope pane |
| `5` | Open Recon pane |
| `i` | Import selected flow's host into scope |
| `Ctrl+h/j/k/l` | Move focus between panes |
| `Ctrl+w s` | Horizontal split |
| `Ctrl+w v` | Vertical split |
| `Ctrl+w c` | Close focused pane |
| `Ctrl+w o` | Close all other panes |
| `Ctrl+w =` | Equalize pane sizes |
| `Ctrl+c` | Quit |

### History pane

| Key | Action |
|-----|--------|
| `enter` | Open flow detail |
| `r` | Open in Repeater |
| `a` | AI analysis |
| `s` | Toggle scope for selected flow's host |
| `f` | Toggle scope filter (show only in-scope / show all) |
| `q` | Quit |

### Scope pane

| Key | Action |
|-----|--------|
| `a` | Add rule (4-step wizard: action → kind → pattern → priority) |
| `d` / `x` | Delete rule |
| `space` / `e` | Toggle rule enabled |
| `/` | Search rules |
| `i` | Import latest captured flow's host |
| `I` | Import all captured flow hosts |
| `q` / `esc` | Close pane |

### Repeater pane

| Key | Action |
|-----|--------|
| `j` / `k` / `tab` | Navigate fields |
| `i` | Edit field |
| `enter` / `s` | Send replay |
| `esc` | Normal mode / back |
| `q` | Close pane |

### Recon pane

| Key | Action |
|-----|--------|
| `enter` | Run recon on target |
| `tab` / `shift+tab` | Switch tabs (Summary, Hosts, Endpoints, Tech, Vulns, AI) |
| `a` | AI analysis of recon results |
| `q` / `esc` | Close pane |

### Command bar (`:`)

| Command | Action |
|---------|--------|
| `:w <name>` | Save current scope rules as a project file |
| `:e <name>` | Load a project's scope rules |
| `:ls` | List saved projects |
| `:q` | Quit |

Projects are stored as JSON in `~/.config/ouroboros/projects/<name>.json`.

---

## LLM Integration

Ouroboros supports multiple AI providers for automated traffic analysis:

```sh
# NVIDIA NIM (free tier available)
export NVIDIA_API_KEY="..."
go run ./cmd/ouroboros --provider=nvidia --model=poolside/laguna-xs-2.1

# Google Gemini
export GEMINI_API_KEY="..."
go run ./cmd/ouroboros --provider=gemini --model=gemini-2.5-flash

# OpenAI
export OPENAI_API_KEY="..."
go run ./cmd/ouroboros --provider=openai --model=gpt-4o-mini

# Local Ollama (100% offline, no API key needed)
go run ./cmd/ouroboros --provider=ollama --model=llama3.2
```

In the TUI, select a captured flow and press `a` to analyze it. For bulk analysis of all in-scope traffic, use the LLM pane's `a` key — it sends every in-scope flow to the model and returns a prioritized findings report.

AI analysis considers your scope rules — only in-scope traffic is sent to the model.

---

## Recon Tools

Ouroboros orchestrates local CLI tools for reconnaissance. Install them and ensure they're in `PATH`:

```sh
# Arch Linux (with BlackArch)
sudo pacman -S subfinder gau waybackurls whatweb exploitdb

# Portable Go installs
go install github.com/projectdiscovery/subfinder/v2/cmd/subfinder@latest
go install github.com/lc/gau/v2/cmd/gau@latest
go install github.com/tomnomnom/waybackurls@latest
```

Open Recon (`5`), enter a target domain, press `enter`. Ouroboros runs all providers in parallel and presents results in tabs:

| Tab | Content |
|-----|--------|
| Summary | Provider status, finding counts |
| Hosts | Discovered subdomains with scope status |
| Endpoints | URLs from gau/waybackurls |
| Tech | Detected technologies (whatweb) |
| Vulns | Known CVEs (searchsploit) |
| AI | LLM-prioritized findings |

---

## Scope Management

Define exactly which hosts are in scope. The scope system enforces boundaries across the entire tool:

- **History filter** — Only in-scope flows appear by default (`f` to toggle)
- **Repeater** — Blocks replay to out-of-scope hosts
- **AI analysis** — Only in-scope flows are sent to the LLM
- **Recon** — Results are filtered to in-scope hosts

Rules support three match modes:
- **Literal** — exact hostname match (e.g., `api.example.com`)
- **Wildcard** — glob pattern (e.g., `*.example.com`)
- **Regex** — anchored regex (prefix with `re:`)

---

## Build & Test

```sh
make fmt        # Format all Go source
make test       # Run unit tests
make test-race  # Run tests with race detector
make run        # Build and run
```

---

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--proxy-addr` | `:8080` | Proxy listen address |
| `--db` | `~/.config/ouroboros/ouroboros.db` | SQLite database path |
| `--memory` | `false` | Use in-memory store (no persistence) |
| `--provider` | auto | LLM provider: `openai`, `ollama`, `nvidia`, `gemini` |
| `--model` | varies | LLM model name |
| `--api-key` | env | LLM API key |
| `--api-base` | provider default | LLM API base URL |
| `--install-ca` | | Print CA cert for browser installation |

---

## Disclaimer

**Authorized Use Only.** Ouroboros is a security testing tool. You MUST only use it against systems you own or have explicit written permission to test. Unauthorized interception of network traffic is illegal in most jurisdictions. The authors assume no liability for misuse.

---

## License

MIT