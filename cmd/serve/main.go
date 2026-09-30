package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/Falasefemi2/jobtracker/internal/db"
	"github.com/Falasefemi2/jobtracker/internal/repo"
	"github.com/Falasefemi2/jobtracker/internal/shortener"
	"github.com/Falasefemi2/jobtracker/internal/web"
)

const shutdownGrace = 10 * time.Second

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	_ = godotenv.Load()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	database, err := db.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Printf("close database: %v", err)
		}
	}()

	baseURL := os.Getenv("BASE_URL")
	addr := listenAddr(baseURL)
	resolver := shortener.New(repo.NewURLRepo(database), baseURL)

	server := &http.Server{
		Addr:              addr,
		Handler:           web.NewHandler(resolver),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errc := make(chan error, 1)
	go func() {
		log.Printf("serving short links on %s (base URL %s)", addr, baseURL)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- err
			return
		}
		errc <- nil
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		log.Print("shutting down")
		return server.Shutdown(shutdownCtx)
	}
}

// listenAddr prefers an explicit ADDR, then the port in BASE_URL, and finally
// the default the shortener also falls back to for link building.
func listenAddr(baseURL string) string {
	if addr := os.Getenv("ADDR"); addr != "" {
		return addr
	}
	if port := portFrom(baseURL); port != "" {
		return ":" + port
	}
	return ":8080"
}

func portFrom(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Port()
}
