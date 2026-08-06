package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"

	"sentinel/internal/intercept"
	"sentinel/internal/llm"
	"sentinel/internal/proxy"
	"sentinel/internal/recon"
	"sentinel/internal/recon/providers/gau"
	"sentinel/internal/recon/providers/subfinder"
	"sentinel/internal/recon/providers/wayback"
	"sentinel/internal/recon/providers/whatweb"
	"sentinel/internal/scope"
	"sentinel/internal/store"
	"sentinel/internal/tui"
)

func main() {
	installCA := flag.Bool("install-ca", false, "Print the CA certificate for browser installation")
	proxyAddr := flag.String("proxy-addr", ":8080", "Proxy listen address")
	providerType := flag.String("provider", "", "LLM provider: openai, ollama, nvidia, or gemini (auto-detects when empty)")
	apiBase := flag.String("api-base", "", "LLM API base URL (e.g. https://integrate.api.nvidia.com/v1)")
	apiKey := flag.String("api-key", "", "LLM API key (defaults to $NVIDIA_API_KEY, $GEMINI_API_KEY, or $OPENAI_API_KEY)")
	model := flag.String("model", "", "LLM model name (e.g. poolside/laguna-xs-2.1, gemini-2.5-flash)")
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

	// Initialize store and scope.
	s := store.NewInMemoryFlowStore()
	sc := scope.NewMatcher([]scope.Rule{
		{Allow: true, Host: regexp.MustCompile(`.*`)},
	})

	// Initialize intercept service (intercept nothing by default).
	is := intercept.NewMatcher(nil)

	// Initialize proxy.
	pxy := proxy.New(s, nil, sc, is)

	// Load or generate CA for MITM.
	ca, err := proxy.LoadOrGenerateCA()
	if err != nil {
		log.Printf("warning: could not load CA, MITM disabled: %v", err)
	} else {
		pxy.SetCA(ca)
	}

	// Initialize TUI with store and proxy reference.
	app := tui.NewAppModel(s, pxy)
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
		// NVIDIA NIM uses an OpenAI-compatible API.
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