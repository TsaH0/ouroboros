# Ouroboros

A terminal web-security workbench for HTTP history inspection, interception, replay, and LLM-assisted analysis.

**Authorized Use Only.** Ouroboros is a security testing tool. You MUST only use it against systems you own or have explicit written permission to test. Unauthorized interception of network traffic is illegal in most jurisdictions. The authors assume no liability for misuse.


## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  TUI (Bubble Tea v2)                                         │
│  ┌─────────────┐ ┌──────────────┐ ┌────────────┐ ┌────────┐ │
│  │ History View │ │ Interceptor  │ │ Repeater   │ │ LLM    │ │
│  └──────┬──────┘ └──────┬───────┘ └──────┬─────┘ └────┬───┘ │
│         │               │                │            │       │
│         ▼               ▼                ▼            ▼       │
│  ┌────────────────────────────────────────────────────────┐ │
│  │  Events & Commands (internal/msg)                      │ │
│  └──────────────────────┬─────────────────────────────────┘ │
└──────────────────────────┼──────────────────────────────────┘
                           │
┌──────────────────────────┼──────────────────────────────────┐
│  Domain Layer            │                                  │
│  ┌───────────────────────▼────────────────────────────────┐ │
│  │  Services (Scope, Intercept Rules, Repeater, LLM)        │ │
│  └───────────────────────┬────────────────────────────────┘ │
│  ┌───────────────────────▼────────────────────────────────┐ │
│  │  Store (FlowStore interface)                             │ │
│  │  ┌──────────────┐  ┌──────────────────────────────┐  │ │
│  │  │ InMemory      │  │ SQLite (future)              │  │ │
│  │  └──────────────┘  └──────────────────────────────┘  │ │
│  └──────────────────────────────────────────────────────┘ │
│  ┌──────────────────────────────────────────────────────┐ │
│  │  Proxy Engine                                          │ │
│  │  ┌──────────────┐  ┌──────────────────────────────┐  │ │
│  │  │ HTTP Handler │  │ MITM (CONNECT, TLS interception)│  │ │
│  │  └──────────────┘  └──────────────────────────────┘  │ │
│  └──────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
```

- **Proxy Engine** — HTTP/HTTPS intercepting forward proxy (Milestone 1+)
- **Domain** — Core models (Flow, Message), scope evaluation, intercept rules
- **Services** — Business logic decoupled from UI and transport
- **Store** — Persistence abstraction (MemoryStore, SQLiteStore with WAL and migrations)
- **Scope** — Include/exclude rules with wildcard, regex, and priority support
- **TUI** — Bubble Tea v2 terminal interface

## Build

Requires Go 1.25+.

```sh
go mod tidy
make run
```

## Commands

| Command | Description |
|---------|-------------|
| `make fmt` | Format all Go source |
| `make test` | Run unit tests |
| `make test-race` | Run tests with race detector |
| `make run` | Build and run |

## TUI usage

History view:

| Key | Action |
|-----|--------|
| `enter` | Open selected flow detail |
| `r` | Open selected flow in Repeater |
| `a` | Open selected flow in LLM analysis |
| `5` | Open Recon Intelligence Workspace |
| `4` | Open Scope Management |
| `q` / `ctrl+c` | Quit |

Repeater view:

| Mode | Key | Action |
|------|-----|--------|
| Normal | `j` / `k` / `tab` / `shift+tab` | Move between Method, URL, Headers, Body, Response |
| Normal | `i` / `enter` | Edit focused request field |
| Normal | `s` / `F5` / `ctrl+j` | Send replay request |
| Normal | `q` / `esc` | Back to history |
| Insert | `esc` | Return to normal mode |
| Insert | `tab` / `shift+tab` | Move to next/previous request field |
| Insert | `s` / `F5` / `ctrl+j` | Send replay request |


Recon workspace:

Ouroboros invokes local reconnaissance CLI tools; it does not bundle or start
them as services. Install the tools and ensure they are in `PATH` before
running Recon:

```sh
# Arch Linux with BlackArch repositories enabled
sudo pacman -S subfinder gau waybackurls whatweb exploitdb

# Verify every provider is available
which subfinder gau waybackurls whatweb searchsploit
```

Portable Go installs for the discovery providers:

```sh
go install github.com/projectdiscovery/subfinder/v2/cmd/subfinder@latest
go install github.com/lc/gau/v2/cmd/gau@latest
go install github.com/tomnomnom/waybackurls@latest
export PATH="$PATH:$(go env GOPATH)/bin"
```

Press `5` from History, enter an authorized target domain, then press `enter`.
The Summary tab reports each provider as `done` or `error` and includes its
finding count. `tab` / `shift+tab` changes tabs, `j` / `k` scrolls, `a` runs
AI prioritization, and `q` / `esc` returns to History.

LLM analysis:

```sh
# NVIDIA NIM
export NVIDIA_API_KEY="..."
go run ./cmd/ouroboros --provider=nvidia --model=poolside/laguna-xs-2.1

# Gemini
export GEMINI_API_KEY="..."
go run ./cmd/ouroboros --provider=gemini --model=gemini-2.5-flash

# OpenAI
export OPENAI_API_KEY="..."
go run ./cmd/ouroboros --provider=openai --model=gpt-4o-mini

# Ollama
go run ./cmd/ouroboros --provider=ollama --model=llama3.2
```

In the TUI, select a captured flow, press `a`, then press `a` again in the LLM view to run analysis.

## Milestone Roadmap

- **Milestone 0** (current) — Foundation: domain models, in-memory store, scope service, Bubble Tea shell with synthetic traffic display (✓ done)
- **Milestone 1** — HTTP forward proxy: plain HTTP interception, request/response capture, flow persistence, history view populated from real traffic (✓ done)
- **Milestone 2** — HTTPS MITM: CONNECT handling, dynamic certificate generation, TLS interception (✓ done)
- **Milestone 3** — Intercept mode: pause flows for modification, forward/drop controls (✓ done)
- **Milestone 4** — Repeater: resend captured requests with editable payloads (✓ done)
- **Milestone 5** — LLM integration: AI-assisted analysis of intercepted traffic (✓ done)
- **Milestone 6** — Persistent Storage + Scope Management: SQLite persistence with WAL, migrations, scope rules with include/exclude/wildcard/regex/priority, scope enforcement in Repeater/Recon/AI, Scope Management TUI screen, history scope badges (✓ done)
- **Milestone 7** — Workspace & Pane Manager: multi-pane workspace with horizontal/vertical splits, Vim/tmux keyboard navigation (Ctrl+h/j/k/l, Ctrl+w s/v/c/o/=), View interface for all screens, status bar, background updates for all panes (✓ done)
