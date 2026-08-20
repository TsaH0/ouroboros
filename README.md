# Ouroboros

<div align="center">

```text
  ___    ___  ___  ___      _
 / _ \ / __\/ _ \/ __\__ _(_)__
| | | / /  | | | / /  / // / _ \
| |_|/ /___| |_|/ /___/ _  /  __/
 \___/\____/\___/\____/_//_/\___|
```

**Terminal-first HTTP intercepting proxy and security workbench.**

[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go)](https://go.dev/)
[![Release](https://img.shields.io/github/v/release/TsaH0/ouroboros)](https://github.com/TsaH0/ouroboros/releases)

</div>

## What it does

Ouroboros captures HTTP/HTTPS traffic, lets you inspect and edit requests, and replays them from one terminal UI. It stores traffic locally in SQLite and keeps scope controls close to every action.

- HTTP and HTTPS MITM proxy on `127.0.0.1:8080`
- HTTP History with all-traffic recording and scope filtering
- Intercept Queue (`3`) with editable floating request editor
- Request Repeater with method, URL, headers, and body editing
- Scope rules and named presets
- Project save/load for engagement scope
- Recon providers: `subfinder`, `gau`, `waybackurls`, `whatweb`, `searchsploit`
- SQLite WAL persistence at `~/.config/ouroboros/ouroboros.db`
- Optional Ouroboros Advisor skill for offline traffic triage through `query.sh`

## Install

Requires Go 1.26+.

```sh
go install github.com/TsaH0/ouroboros/cmd/ouroboros@latest
```

Ensure `$GOBIN` (normally `$(go env GOPATH)/bin`) is in `PATH`, then run:

```sh
ouroboros
```

Or download source:

```sh
git clone https://github.com/TsaH0/ouroboros.git
cd ouroboros
make build
./bin/ouroboros
```

## HTTPS setup

Generate the Ouroboros CA certificate:

```sh
ouroboros --install-ca > /tmp/ouroboros-ca.pem
```

Install `/tmp/ouroboros-ca.pem` into your OS/browser trust store. On Arch Linux, OS trust uses:

```sh
sudo cp /tmp/ouroboros-ca.pem /etc/ca-certificates/trust-source/anchors/ouroboros.crt
sudo update-ca-trust
```

Firefox and Chromium-based browsers may need separate certificate-store configuration. Never share the generated CA private key.

Set browser proxy to `127.0.0.1:8080`, then browse. Ouroboros records flows automatically.

## Advisor skill

Go installs binaries, not agent skills. Ouroboros therefore does **not** silently write files into a user's home directory during normal startup. Install skill explicitly:

```sh
ouroboros --install-skill
```

Default destination:

```text
~/.agents/skills/ouroboros-advisor/
├── SKILL.md
└── scripts/query.sh
```

Custom destination:

```sh
ouroboros --install-skill --skill-dir ~/.gemini/antigravity-cli/skills/ouroboros-advisor
```

The skill reads local database only. It requires `sqlite3`; `jq` is useful for JSON output:

```sh
sudo pacman -S sqlite jq
bash ~/.agents/skills/ouroboros-advisor/scripts/query.sh overview
bash ~/.agents/skills/ouroboros-advisor/scripts/query.sh hosts
bash ~/.agents/skills/ouroboros-advisor/scripts/query.sh triage
bash ~/.agents/skills/ouroboros-advisor/scripts/query.sh flow <flow-id>
```

Use `OUROBOROS_DB=/path/to/ouroboros.db` to inspect another database.

## TUI quick reference

| Key | Action |
|---|---|
| `0` | HTTP History |
| `3` | Intercept Queue |
| `4` | Scope manager |
| `5` | Recon |
| `I` | Toggle interception |
| `enter` | Open selected flow |
| `r` | Open Repeater |
| `s` | Toggle selected host scope in current session |
| `f` | Filter history to in-scope flows |
| `a` | Add scope rule |
| `C` | Clear displayed history, keep database |
| `D` | Wipe persisted history after confirmation |
| `q` / `esc` | Back or close floating pane |

Intercept workflow: press `I`, open queue with `3`, select flow, press `enter`, edit with `e`, then `f` to forward or `d` to drop. Paste works with bracketed terminal paste (`Ctrl+Shift+V`, terminal-dependent).

## Scope safety

Scope defaults to all hosts so traffic remains visible. Repeater enforces scope before sending. Use `4` to create explicit host, path, URL, wildcard, or regex rules. `s` in History is an in-memory session toggle; use `a` in Scope when you want a persisted rule.

Only test systems you own or have permission to test. PortSwigger Web Security Academy labs are suitable authorized targets.

## Data and privacy

- Database: `~/.config/ouroboros/ouroboros.db`
- CA: `~/.config/ouroboros/ca.pem`
- No cloud service required for proxy, storage, scope, or triage
- Captured cookies, authorization headers, and request bodies may contain secrets; protect the database

## Development

```sh
go test ./...
go vet ./...
make build
```

Architecture: Go, Bubble Tea v2, Bubbles v2, Lip Gloss v2, SQLite WAL. See `skills/ouroboros-advisor/SKILL.md` for the complete progressive triage workflow.

## License

See repository license and release notes.
