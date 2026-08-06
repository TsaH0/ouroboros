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
	"sentinel/internal/proxy"
	"sentinel/internal/scope"
	"sentinel/internal/store"
	"sentinel/internal/tui"
)

func main() {
	installCA := flag.Bool("install-ca", false, "Print the CA certificate for browser installation")
	proxyAddr := flag.String("proxy-addr", ":8080", "Proxy listen address")
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
