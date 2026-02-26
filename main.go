package main

import (
	"context"
	"embed"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	"github.com/ningw42/nixpkgs-pr-tracker/internal/config"
	"github.com/ningw42/nixpkgs-pr-tracker/internal/db"
	"github.com/ningw42/nixpkgs-pr-tracker/internal/event"
	"github.com/ningw42/nixpkgs-pr-tracker/internal/github"
	"github.com/ningw42/nixpkgs-pr-tracker/internal/notifier"
	"github.com/ningw42/nixpkgs-pr-tracker/internal/poller"
	"github.com/ningw42/nixpkgs-pr-tracker/internal/server"
)

//go:embed web/templates/*
var templateFS embed.FS

func main() {
	cfg := config.Load()

	database, err := db.New(cfg.DBPath)
	if err != nil {
		log.Fatalf("opening database: %v", err)
	}
	defer database.Close()

	ghClient := github.New(cfg.GitHubToken)
	bus := event.New()

	// Register notifiers
	if cfg.WebhookURL != "" {
		wh := notifier.NewWebhook(cfg.WebhookURL)
		bus.Subscribe(func(e event.Event) {
			if err := wh.Notify(context.Background(), e); err != nil {
				log.Printf("webhook error: %v", err)
			}
		})
		if u, err := url.Parse(cfg.WebhookURL); err == nil {
			log.Printf("webhook notifier enabled: %s://%s/***", u.Scheme, u.Host)
		} else {
			log.Printf("webhook notifier enabled")
		}
	} else {
		log.Printf("webhook notifier disabled (NPT_WEBHOOK_URL not set)")
	}

	// Start poller
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	p := poller.New(database, ghClient, bus, cfg.PollInterval, cfg.Branches)
	p.Start(ctx)
	log.Printf("poller started (interval: %s, branches: %v)", cfg.PollInterval, cfg.Branches)

	// Parse templates
	tmpl := template.Must(template.ParseFS(templateFS, "web/templates/*.html"))

	// Start HTTP server
	srv := server.New(database, ghClient, bus, cfg.Branches, tmpl)
	httpServer := &http.Server{Addr: cfg.ListenAddr, Handler: srv.Routes()}

	go func() {
		<-ctx.Done()
		log.Println("shutting down...")
		httpServer.Shutdown(context.Background())
	}()

	log.Printf("listening on %s", cfg.ListenAddr)
	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("http server: %v", err)
	}
}
