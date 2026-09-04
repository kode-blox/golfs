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

// Package config loads and validates the GOLFS environment contract.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Runtime defaults and hard protocol limits.
const (
	DefaultHTTPAddress      = ":8080"
	DefaultAdminAddress     = ":9090"
	DefaultPresignTTL       = time.Hour
	DefaultMaxObjectSize    = int64(5_000_000_000)
	MaximumMaxObjectSize    = int64(5_000_000_000)
	AuthorizationCacheTTL   = time.Minute
	AuthorizationCacheSize  = 10_000
	MaximumBatchObjects     = 1_000
	MaximumBatchRequestSize = 10 << 20
	ObjectCheckConcurrency  = 16
)

// Config contains all runtime configuration. Secrets are populated through
// environment variables so Kubernetes operators can use ConfigMaps and Secrets.
type Config struct {
	PublicURL string

	HTTPAddress  string
	AdminAddress string

	GitHubAppClientID      string
	GitHubAppPrivateKeyPEM string

	S3Endpoint     string
	S3Region       string
	S3Bucket       string
	S3UsePathStyle bool
	PresignTTL     time.Duration

	MaxObjectSize int64
	LogLevel      slog.Level
	AllowHTTP     bool
}

// Load reads and validates the public environment-variable contract.
func Load() (Config, error) {
	cfg := Config{
		PublicURL:              strings.TrimRight(os.Getenv("GOLFS_PUBLIC_URL"), "/"),
		HTTPAddress:            envOrDefault("GOLFS_HTTP_ADDR", DefaultHTTPAddress),
		AdminAddress:           envOrDefault("GOLFS_ADMIN_ADDR", DefaultAdminAddress),
		GitHubAppClientID:      os.Getenv("GOLFS_GITHUB_APP_CLIENT_ID"),
		GitHubAppPrivateKeyPEM: os.Getenv("GOLFS_GITHUB_APP_PRIVATE_KEY_PEM"),
		S3Endpoint:             strings.TrimRight(os.Getenv("GOLFS_S3_ENDPOINT"), "/"),
		S3Region:               os.Getenv("GOLFS_S3_REGION"),
		S3Bucket:               os.Getenv("GOLFS_S3_BUCKET"),
		PresignTTL:             DefaultPresignTTL,
		MaxObjectSize:          DefaultMaxObjectSize,
		LogLevel:               slog.LevelInfo,
	}

	var err error
	if cfg.AllowHTTP, err = envBool("GOLFS_DEV_ALLOW_HTTP", false); err != nil {
		return Config{}, err
	}
	if cfg.S3UsePathStyle, err = envBool("GOLFS_S3_USE_PATH_STYLE", false); err != nil {
		return Config{}, err
	}
	if cfg.PresignTTL, err = envDuration("GOLFS_PRESIGN_TTL", DefaultPresignTTL); err != nil {
		return Config{}, err
	}
	if cfg.MaxObjectSize, err = envInt64("GOLFS_MAX_OBJECT_SIZE", DefaultMaxObjectSize); err != nil {
		return Config{}, err
	}
	if cfg.LogLevel, err = parseLogLevel(envOrDefault("GOLFS_LOG_LEVEL", "info")); err != nil {
		return Config{}, err
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate fails closed on ambiguous or unsafe production configuration.
func (c Config) Validate() error {
	required := map[string]string{
		"GOLFS_PUBLIC_URL":                 c.PublicURL,
		"GOLFS_GITHUB_APP_CLIENT_ID":       c.GitHubAppClientID,
		"GOLFS_GITHUB_APP_PRIVATE_KEY_PEM": c.GitHubAppPrivateKeyPEM,
		"GOLFS_S3_ENDPOINT":                c.S3Endpoint,
		"GOLFS_S3_REGION":                  c.S3Region,
		"GOLFS_S3_BUCKET":                  c.S3Bucket,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	if c.HTTPAddress == "" || c.AdminAddress == "" {
		return errors.New("HTTP listener addresses must not be empty")
	}
	if c.HTTPAddress == c.AdminAddress {
		return errors.New("GOLFS_HTTP_ADDR and GOLFS_ADMIN_ADDR must differ")
	}
	if _, _, err := net.SplitHostPort(c.HTTPAddress); err != nil {
		return fmt.Errorf("GOLFS_HTTP_ADDR must be a host:port listener address: %w", err)
	}
	if _, _, err := net.SplitHostPort(c.AdminAddress); err != nil {
		return fmt.Errorf("GOLFS_ADMIN_ADDR must be a host:port listener address: %w", err)
	}

	publicURL, err := url.Parse(c.PublicURL)
	if err != nil {
		return fmt.Errorf("parse GOLFS_PUBLIC_URL: %w", err)
	}
	if publicURL.Scheme == "" || publicURL.Host == "" || publicURL.User != nil || publicURL.RawQuery != "" || publicURL.Fragment != "" {
		return errors.New("GOLFS_PUBLIC_URL must be an absolute origin without credentials, query, or fragment")
	}
	if publicURL.Path != "" && publicURL.Path != "/" {
		return errors.New("GOLFS_PUBLIC_URL must not contain a path")
	}
	if publicURL.Scheme != "https" && (!c.AllowHTTP || publicURL.Scheme != "http") {
		return errors.New("GOLFS_PUBLIC_URL must use HTTPS unless GOLFS_DEV_ALLOW_HTTP=true")
	}

	s3URL, err := url.Parse(c.S3Endpoint)
	if err != nil || s3URL.Scheme == "" || s3URL.Host == "" {
		return errors.New("GOLFS_S3_ENDPOINT must be an absolute URL")
	}
	if s3URL.User != nil || s3URL.RawQuery != "" || s3URL.Fragment != "" {
		return errors.New("GOLFS_S3_ENDPOINT must not contain credentials, a query, or a fragment")
	}
	if s3URL.Scheme != "https" && (!c.AllowHTTP || s3URL.Scheme != "http") {
		return errors.New("GOLFS_S3_ENDPOINT must use HTTPS unless GOLFS_DEV_ALLOW_HTTP=true")
	}
	if c.PresignTTL <= 0 || c.PresignTTL > 7*24*time.Hour {
		return errors.New("GOLFS_PRESIGN_TTL must be greater than zero and at most 168h")
	}
	if c.MaxObjectSize <= 0 || c.MaxObjectSize > MaximumMaxObjectSize {
		return fmt.Errorf("GOLFS_MAX_OBJECT_SIZE must be between 1 and %d", MaximumMaxObjectSize)
	}
	return nil
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envBool(name string, fallback bool) (bool, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", name, err)
	}
	return parsed, nil
}

func envDuration(name string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	return parsed, nil
}

func envInt64(name string, fallback int64) (int64, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", name, err)
	}
	return parsed, nil
}

func parseLogLevel(value string) (slog.Level, error) {
	switch strings.ToLower(value) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid GOLFS_LOG_LEVEL %q", value)
	}
}
