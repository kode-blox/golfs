// Copyright 2026 Sayak Mukhopadhyay
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Command golfs runs the public Git LFS and cluster-internal operations listeners.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/kode-blox/golfs/internal/authcache"
	"github.com/kode-blox/golfs/internal/config"
	githubforge "github.com/kode-blox/golfs/internal/forge/github"
	"github.com/kode-blox/golfs/internal/lfs"
	"github.com/kode-blox/golfs/internal/observability"
	"github.com/kode-blox/golfs/internal/storage/s3store"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	if err := run(); err != nil {
		slog.Error("GOLFS stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)
	metrics := observability.New(version, commit)

	forgeAuthorizer, err := githubforge.New(githubforge.Config{
		ClientID: cfg.GitHubAppClientID, PrivateKeyPEM: cfg.GitHubAppPrivateKeyPEM,
		Metrics: metrics,
	})
	if err != nil {
		return err
	}
	objectStore, err := s3store.New(context.Background(), s3store.Config{
		Endpoint: cfg.S3Endpoint, Region: cfg.S3Region, Bucket: cfg.S3Bucket,
		UsePathStyle: cfg.S3UsePathStyle, PresignTTL: cfg.PresignTTL, Metrics: metrics,
	})
	if err != nil {
		return err
	}
	startupContext, cancelStartup := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelStartup()
	if err := forgeAuthorizer.Validate(startupContext); err != nil {
		return err
	}
	if err := objectStore.Validate(startupContext); err != nil {
		return err
	}
	cache, err := authcache.New(config.AuthorizationCacheSize, config.AuthorizationCacheTTL)
	if err != nil {
		return err
	}

	protocol := lfs.New(lfs.Options{
		PublicURL: cfg.PublicURL, MaxObjectSize: cfg.MaxObjectSize,
		Authorizer: forgeAuthorizer, Cache: cache, Store: objectStore,
		Metrics: metrics, Logger: logger,
	})
	publicMux := http.NewServeMux()
	protocol.Register(publicMux)
	publicHandler := routeMetrics(metrics, "public", lfs.Middleware(logger, publicMux))

	var ready atomic.Bool
	adminMux := http.NewServeMux()
	adminMux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	adminMux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !ready.Load() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready\n"))
	})
	adminMux.Handle("GET /metrics", promhttp.HandlerFor(metrics.Registry, promhttp.HandlerOpts{}))
	adminHandler := routeMetrics(metrics, "operations", lfs.Middleware(logger, adminMux))

	publicServer := server(cfg.HTTPAddress, publicHandler)
	adminServer := server(cfg.AdminAddress, adminHandler)
	serverErrors := make(chan error, 2)
	go serve(logger, "public", publicServer, serverErrors)
	go serve(logger, "operations", adminServer, serverErrors)
	ready.Store(true)
	logger.Info("GOLFS ready", "version", version, "commit", commit, "public_addr", cfg.HTTPAddress, "admin_addr", cfg.AdminAddress)

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	var runError error
	select {
	case received := <-signals:
		logger.Info("shutdown requested", "signal", received.String())
	case runError = <-serverErrors:
	}
	ready.Store(false)
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancelShutdown()
	publicError := publicServer.Shutdown(shutdownContext)
	adminError := adminServer.Shutdown(shutdownContext)
	return errors.Join(runError, publicError, adminError)
}

func server(address string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr: address, Handler: handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
	}
}

func serve(logger *slog.Logger, name string, server *http.Server, errorsChannel chan<- error) {
	logger.Info("listener started", "listener", name, "address", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		errorsChannel <- err
	}
}

func routeMetrics(metrics *observability.Metrics, listener string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := "not_found"
		switch {
		case strings.HasSuffix(r.URL.Path, "/info/lfs/objects/batch"):
			route = "batch"
		case strings.HasSuffix(r.URL.Path, "/info/lfs/objects/verify"):
			route = "verify"
		case r.URL.Path == "/healthz":
			route = "healthz"
		case r.URL.Path == "/readyz":
			route = "readyz"
		case r.URL.Path == "/metrics":
			route = "metrics"
		}
		metrics.Instrument(listener, route, next).ServeHTTP(w, r)
	})
}
