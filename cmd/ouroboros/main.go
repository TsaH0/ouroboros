package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"

	"ouroboros/internal/intercept"
	"ouroboros/internal/proxy"
	"ouroboros/internal/recon"
	"ouroboros/internal/recon/providers/gau"
	"ouroboros/internal/recon/providers/searchsploit"
	"ouroboros/internal/recon/providers/subfinder"
	"ouroboros/internal/recon/providers/wayback"
	"ouroboros/internal/recon/providers/whatweb"
	"ouroboros/internal/scope"
	"ouroboros/internal/store"
	"ouroboros/internal/tui"
)

func main() {
	installCA := flag.Bool("install-ca", false, "Print the CA certificate for browser installation")
	proxyAddr := flag.String("proxy-addr", ":8080", "Proxy listen address")
	dbPath := flag.String("db", "", "SQLite database path (default: ~/.config/ouroboros/ouroboros.db)")
	memory := flag.Bool("memory", false, "Use in-memory store instead of SQLite")
	flag.Parse()

	if *installCA {
		if err := proxy.PrintCACert(); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Initialize store.
	var st store.Store
	if *memory {
		st = store.NewMemoryStore()
	} else {
		path := *dbPath
		if path == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				log.Fatalf("home dir: %v", err)
			}
			dir := filepath.Join(home, ".config", "ouroboros")
			if err := os.MkdirAll(dir, 0700); err != nil {
				log.Fatalf("create config dir: %v", err)
			}
			path = filepath.Join(dir, "ouroboros.db")
		}
		sqlStore, err := store.NewSQLiteStore(path)
		if err != nil {
			log.Fatalf("open database: %v", err)
		}
		st = sqlStore
	}
	defer st.Close()

	// Initialize scope manager.
	scopeMgr := scope.NewManager(st)
	if err := scopeMgr.Load(ctx); err != nil {
		log.Printf("warning: load scope rules: %v", err)
	}
	// Default wildcard scope: ensure '*' (all hosts) is in scope by default
	// so all websites are in scope unless explicitly excluded.
	if len(scopeMgr.Rules()) == 0 {
		_, _ = scopeMgr.AddRule(ctx, scope.Rule{
			Kind:      scope.RuleKindHost,
			Pattern:   "*",
			MatchMode: scope.MatchModeWildcard,
			Action:    scope.ActionInclude,
			Enabled:   true,
			Priority:  0,
			Note:      "Default wildcard scope (* all hosts)",
		})
	}

	// Initialize intercept service (intercept nothing by default).
	is := intercept.NewMatcher(nil)

	// Initialize proxy.
	pxy := proxy.New(st, nil, scopeMgr, is)

	// Load or generate CA for MITM.
	ca, err := proxy.LoadOrGenerateCA()
	if err != nil {
		log.Printf("warning: could not load CA, MITM disabled: %v", err)
	} else {
		pxy.SetCA(ca)
	}

	// Initialize TUI with store, proxy, and scope manager.
	app := tui.NewAppModel(st, pxy, scopeMgr)
	p := tea.NewProgram(app, tea.WithContext(ctx))

	// Wire the program into the proxy so it can send events.
	pxy.SetProgram(p)

	// Initialize recon engine.
	reconCache := recon.NewCache()
	reconRunner := recon.DefaultCommandRunner{}
	reconProviders := []recon.ProviderMetadata{
		{Provider: &subfinder.Provider{Runner: reconRunner}, Role: recon.RoleDiscovery, Timeout: 60},
		{Provider: &gau.Provider{Runner: reconRunner}, Role: recon.RoleDiscovery, Timeout: 60},
		{Provider: &wayback.Provider{Runner: reconRunner}, Role: recon.RoleDiscovery, Timeout: 60},
		{Provider: &whatweb.Provider{Runner: reconRunner}, Role: recon.RoleEnrichment, Timeout: 30},
		{Provider: &searchsploit.Provider{Runner: reconRunner}, Role: recon.RoleEnrichment, Timeout: 30},
	}
	reconEngine := recon.NewEngine(reconCache, reconRunner, reconProviders)
	app.SetReconEngine(reconEngine)

	// LLM analysis is intentionally decoupled from the TUI.
	// Use skills/ouroboros-advisor/scripts/query.sh (or the global skill) for
	// AI-assisted traffic/recon triage:
	//   bash skills/ouroboros-advisor/scripts/query.sh triage
	//   bash skills/ouroboros-advisor/scripts/query.sh flow <id>

	// Start proxy server.
	proxyServer := &http.Server{
		Addr:    *proxyAddr,
		Handler: pxy,
	}
	go func() {
		log.Printf("proxy listening on %s", *proxyAddr)
		if err := proxyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("proxy error: %v", err)
		}
	}()

	// Run TUI (blocks until quit).
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "tui error: %v\n", err)
	}

	// Shutdown proxy.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	proxyServer.Shutdown(shutdownCtx)
}
