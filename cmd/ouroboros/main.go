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
	"ouroboros/internal/llm"
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
	providerType := flag.String("provider", "", "LLM provider: openai, ollama, nvidia, or gemini (auto-detects when empty)")
	apiBase := flag.String("api-base", "", "LLM API base URL (e.g. https://integrate.api.nvidia.com/v1)")
	apiKey := flag.String("api-key", "", "LLM API key (defaults to $NVIDIA_API_KEY, $GEMINI_API_KEY, or $OPENAI_API_KEY)")
	model := flag.String("model", "", "LLM model name (e.g. poolside/laguna-xs-2.1, gemini-2.5-flash)")
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
	// Seed a default allow-all rule if no rules exist.
	if len(scopeMgr.Rules()) == 0 {
		_, err := scopeMgr.AddRule(ctx, scope.Rule{
			Kind:      scope.RuleKindHost,
			Pattern:   "*",
			MatchMode: scope.MatchModeWildcard,
			Action:    scope.ActionInclude,
			Enabled:   true,
			Priority:  0,
			Note:      "default allow-all",
		})
		if err != nil {
			log.Printf("warning: seed default scope rule: %v", err)
		}
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

	// Configure LLM provider.
	providerName := *providerType
	if providerName == "" {
		switch {
		case os.Getenv("NVIDIA_API_KEY") != "":
			providerName = "nvidia"
		case os.Getenv("GEMINI_API_KEY") != "":
			providerName = "gemini"
		case os.Getenv("OPENAI_API_KEY") != "":
			providerName = "openai"
		default:
			providerName = "ollama"
		}
	}

	pt := llm.ProviderOpenAI
	switch providerName {
	case "ollama":
		pt = llm.ProviderOllama
	case "nvidia":
		pt = llm.ProviderOpenAI
		if *apiBase == "" {
			*apiBase = "https://integrate.api.nvidia.com/v1"
		}
		if *apiKey == "" {
			*apiKey = os.Getenv("NVIDIA_API_KEY")
		}
		if *model == "" {
			*model = "poolside/laguna-xs-2.1"
		}
	case "gemini":
		pt = llm.ProviderGemini
		if *apiKey == "" {
			*apiKey = os.Getenv("GEMINI_API_KEY")
		}
		if *model == "" {
			*model = "gemini-2.5-flash"
		}
	case "openai":
		pt = llm.ProviderOpenAI
	default:
		log.Fatalf("unknown provider: %s (use openai, ollama, nvidia, or gemini)", providerName)
	}

	provider, modelName := llm.NewProvider(pt, *apiBase, *apiKey, *model)
	if provider != nil {
		analyzer := llm.NewAnalyzer(provider, modelName)
		app.SetAnalyzer(analyzer)
		log.Printf("LLM provider: %s, model: %s", providerName, modelName)
	} else {
		log.Println("no LLM provider configured (use --provider, --api-base, --api-key, --model)")
	}

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
