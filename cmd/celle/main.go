// Command celle serves the celle API.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // the container has no zoneinfo

	"github.com/deviationist/celle/internal/api"
	"github.com/deviationist/celle/internal/calendar"
	"github.com/deviationist/celle/internal/dates"
)

var version = "dev"

func main() {
	addr := env("CELLE_ADDR", ":8080")
	if len(os.Args) > 1 && os.Args[1] == "-health" {
		os.Exit(healthProbe(addr))
	}
	tzName := env("CELLE_DEFAULT_TZ", "Europe/Oslo")
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		slog.Error("bad CELLE_DEFAULT_TZ", "tz", tzName, "err", err)
		os.Exit(1)
	}

	tokens := api.ParseTokens(os.Getenv("CELLE_TOKENS"))
	if len(tokens) == 0 {
		slog.Error("CELLE_TOKENS is empty: set at least one token (openssl rand -hex 24)")
		os.Exit(1)
	}
	for _, t := range tokens {
		if len(t) < 16 {
			slog.Error("every token in CELLE_TOKENS must be at least 16 characters")
			os.Exit(1)
		}
	}

	payday := os.Getenv("CELLE_DEFAULT_PAYDAY")
	if payday != "" {
		if _, err := dates.ParsePayday(payday); err != nil {
			slog.Error("bad CELLE_DEFAULT_PAYDAY", "err", err)
			os.Exit(1)
		}
	}
	country := os.Getenv("CELLE_DEFAULT_COUNTRY")
	if country != "" {
		if _, err := calendar.Get(country); err != nil {
			slog.Error("bad CELLE_DEFAULT_COUNTRY", "err", err)
			os.Exit(1)
		}
	}

	srv := &http.Server{
		Addr: addr,
		Handler: api.New(api.Config{
			Version: version, DefaultTZ: loc, Tokens: tokens,
			DefaultPayday: payday, DefaultCountry: country,
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		slog.Info("listening", "addr", addr, "version", version, "default_tz", tzName, "tokens", len(tokens), "default_payday", payday, "default_country", country)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server", "err", err)
			os.Exit(1)
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// healthProbe GETs /healthz on the local listener, for the container
// healthcheck (the image has no curl).
func healthProbe(addr string) int {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 1
	}
	c := http.Client{Timeout: 2 * time.Second}
	res, err := c.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		return 1
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
