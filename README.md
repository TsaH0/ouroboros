# Sentinel

A terminal web-security workbench for HTTP history inspection, interception, replay, and LLM-assisted analysis.

**Authorized Use Only.** Sentinel is a security testing tool. You MUST only use it against systems you own or have explicit written permission to test. Unauthorized interception of network traffic is illegal in most jurisdictions. The authors assume no liability for misuse.

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
- **Store** — Persistence abstraction (in-memory now, SQLite later)
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

## Milestone Roadmap

- **Milestone 0** (current) — Foundation: domain models, in-memory store, scope service, Bubble Tea shell with synthetic traffic display (✓ done)
- **Milestone 1** — HTTP forward proxy: plain HTTP interception, request/response capture, flow persistence, history view populated from real traffic (✓ done)
- **Milestone 2** — HTTPS MITM: CONNECT handling, dynamic certificate generation, TLS interception (✓ done)
- **Milestone 3** — Intercept mode: pause flows for modification, forward/drop controls (✓ done)
- **Milestone 4** — Repeater: resend captured requests with editable payloads (✓ done)
- **Milestone 5** — LLM integration: AI-assisted analysis of intercepted traffic (✓ done)
