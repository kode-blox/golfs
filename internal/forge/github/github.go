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

// Package github implements GitHub App repository authorization.
package github

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kode-blox/golfs/internal/forge"
	"github.com/kode-blox/golfs/internal/observability"
)

const (
	defaultAPIURL = "https://api.github.com"
	apiVersion    = "2026-03-10"
)

// Config supplies GitHub App credentials and injectable test dependencies.
type Config struct {
	ClientID      string
	PrivateKeyPEM string
	APIURL        string
	HTTPClient    *http.Client
	Metrics       *observability.Metrics
}

// Authorizer validates GitHub App user tokens and resolves repository permissions.
type Authorizer struct {
	clientID   string
	privateKey *rsa.PrivateKey
	baseURL    *url.URL
	client     *http.Client
	metrics    *observability.Metrics

	jwtMu      sync.Mutex
	appJWT     string
	jwtExpires time.Time
	now        func() time.Time
}

// New parses App credentials and constructs a GitHub authorizer.
func New(cfg Config) (*Authorizer, error) {
	privateKey, err := parsePrivateKey([]byte(cfg.PrivateKeyPEM))
	if err != nil {
		return nil, err
	}
	apiURL := cfg.APIURL
	if apiURL == "" {
		apiURL = defaultAPIURL
	}
	baseURL, err := url.Parse(strings.TrimRight(apiURL, "/"))
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, errors.New("GitHub API URL must be absolute")
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	if strings.TrimSpace(cfg.ClientID) == "" {
		return nil, errors.New("GitHub App client ID is required")
	}
	return &Authorizer{
		clientID: cfg.ClientID, privateKey: privateKey, baseURL: baseURL,
		client: client, metrics: cfg.Metrics, now: time.Now,
	}, nil
}

// Validate authenticates the configured App during startup.
func (a *Authorizer) Validate(ctx context.Context) error {
	var app struct {
		ClientID string `json:"client_id"`
	}
	if err := a.request(ctx, http.MethodGet, "/app", "app_validation", a.appAuthorization, &app); err != nil {
		return fmt.Errorf("validate GitHub App credentials: %w", err)
	}
	if app.ClientID != "" && app.ClientID != a.clientID {
		return errors.New("GitHub authenticated App client ID does not match configuration")
	}
	return nil
}

// Authorize proves App installation membership and resolves the user's repository permission.
func (a *Authorizer) Authorize(ctx context.Context, owner, repository, token string) (forge.Authorization, error) {
	// GitHub App user access tokens use the ghu_ prefix. Rejecting all other
	// token classes prevents PAT and installation-token fallback.
	if !strings.HasPrefix(token, "ghu_") {
		return forge.Authorization{}, forge.ErrUnauthenticated
	}

	path := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repository) + "/installation"
	var installation struct {
		ID       int64  `json:"id"`
		ClientID string `json:"client_id"`
		AppID    int64  `json:"app_id"`
	}
	if err := a.request(ctx, http.MethodGet, path, "repository_installation", a.appAuthorization, &installation); err != nil {
		return forge.Authorization{}, err
	}
	if installation.ID <= 0 {
		return forge.Authorization{}, forge.ErrNotFound
	}
	if installation.ClientID != "" && installation.ClientID != a.clientID {
		return forge.Authorization{}, forge.ErrNotFound
	}

	matched, err := a.userCanAccessInstallation(ctx, token, installation.ID)
	if err != nil {
		return forge.Authorization{}, err
	}
	if !matched {
		return forge.Authorization{}, forge.ErrUnauthenticated
	}

	var repo struct {
		ID          int64  `json:"id"`
		FullName    string `json:"full_name"`
		Permissions struct {
			Pull bool `json:"pull"`
			Push bool `json:"push"`
		} `json:"permissions"`
	}
	if err := a.request(ctx, http.MethodGet, path[:strings.LastIndex(path, "/installation")], "repository", func() (string, error) { return "Bearer " + token, nil }, &repo); err != nil {
		return forge.Authorization{}, err
	}
	if repo.ID <= 0 || repo.FullName == "" {
		return forge.Authorization{}, forge.ErrNotFound
	}
	permission := forge.PermissionNone
	if repo.Permissions.Pull {
		permission = forge.PermissionRead
	}
	if repo.Permissions.Push {
		permission = forge.PermissionWrite
	}
	return forge.Authorization{RepositoryID: repo.ID, Permission: permission, CanonicalName: repo.FullName}, nil
}

func (a *Authorizer) userCanAccessInstallation(ctx context.Context, token string, target int64) (bool, error) {
	path := "/user/installations?per_page=100"
	for page := 0; page < 100; page++ {
		var response struct {
			Installations []struct {
				ID int64 `json:"id"`
			} `json:"installations"`
		}
		next, err := a.requestWithNext(ctx, http.MethodGet, path, "user_installations", func() (string, error) { return "Bearer " + token, nil }, &response)
		if err != nil {
			return false, err
		}
		for _, installation := range response.Installations {
			if installation.ID == target {
				return true, nil
			}
		}
		if next == "" {
			return false, nil
		}
		path = next
	}
	return false, forge.ErrUnavailable
}

type authorization func() (string, error)

func (a *Authorizer) appAuthorization() (string, error) {
	value, err := a.signedAppJWT()
	if err != nil {
		return "", err
	}
	return "Bearer " + value, nil
}

func (a *Authorizer) request(ctx context.Context, method, path, operation string, auth authorization, target any) error {
	_, err := a.requestWithNext(ctx, method, path, operation, auth, target)
	return err
}

func (a *Authorizer) requestWithNext(ctx context.Context, method, path, operation string, auth authorization, target any) (string, error) {
	endpoint := path
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = a.baseURL.String() + path
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return "", forge.ErrUnavailable
	}
	authorization, err := auth()
	if err != nil {
		return "", fmt.Errorf("create GitHub authorization: %w", err)
	}
	request.Header.Set("Authorization", authorization)
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", apiVersion)
	request.Header.Set("User-Agent", "GOLFS")

	started := time.Now()
	response, err := a.client.Do(request)
	if err != nil {
		a.observe(operation, "network_error", started)
		return "", forge.ErrUnavailable
	}
	defer func() { _ = response.Body.Close() }()
	a.observe(operation, strconv.Itoa(response.StatusCode), started)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return "", mapResponseError(response)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 4<<20))
	if err := decoder.Decode(target); err != nil {
		return "", forge.ErrUnavailable
	}
	return parseNext(response.Header.Get("Link")), nil
}

func (a *Authorizer) observe(operation, status string, started time.Time) {
	if a.metrics == nil {
		return
	}
	a.metrics.GitHubRequests.WithLabelValues(operation, status).Inc()
	a.metrics.GitHubDuration.WithLabelValues(operation).Observe(time.Since(started).Seconds())
}

func mapResponseError(response *http.Response) error {
	switch response.StatusCode {
	case http.StatusUnauthorized:
		return forge.ErrUnauthenticated
	case http.StatusForbidden:
		if response.Header.Get("X-RateLimit-Remaining") == "0" {
			return retryAfterError(response)
		}
		return forge.ErrForbidden
	case http.StatusNotFound:
		return forge.ErrNotFound
	case http.StatusTooManyRequests:
		return retryAfterError(response)
	default:
		if response.StatusCode >= 500 {
			return forge.ErrUnavailable
		}
		return forge.ErrUnauthenticated
	}
}

func retryAfterError(response *http.Response) error {
	delay := time.Minute
	if seconds, err := strconv.Atoi(response.Header.Get("Retry-After")); err == nil && seconds > 0 {
		delay = time.Duration(seconds) * time.Second
	} else if reset, err := strconv.ParseInt(response.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
		if candidate := time.Until(time.Unix(reset, 0)); candidate > 0 {
			delay = candidate
		}
	}
	return &forge.RetryAfterError{Err: forge.ErrRateLimited, RetryAfter: delay}
}

func parseNext(link string) string {
	for _, part := range strings.Split(link, ",") {
		segments := strings.Split(part, ";")
		if len(segments) < 2 || !strings.Contains(segments[1], `rel="next"`) {
			continue
		}
		return strings.Trim(strings.TrimSpace(segments[0]), "<>")
	}
	return ""
}

func (a *Authorizer) signedAppJWT() (string, error) {
	a.jwtMu.Lock()
	defer a.jwtMu.Unlock()
	now := a.now()
	if a.appJWT != "" && now.Add(time.Minute).Before(a.jwtExpires) {
		return a.appJWT, nil
	}
	expires := now.Add(9 * time.Minute)
	claims := jwt.RegisteredClaims{
		Issuer:    a.clientID,
		IssuedAt:  jwt.NewNumericDate(now.Add(-60 * time.Second)),
		ExpiresAt: jwt.NewNumericDate(expires),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := token.SignedString(a.privateKey)
	if err != nil {
		return "", fmt.Errorf("sign GitHub App JWT: %w", err)
	}
	a.appJWT, a.jwtExpires = signed, expires
	return signed, nil
}

func parsePrivateKey(data []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("parse GitHub App private key: no PEM block found")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse GitHub App private key: %w", err)
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("GitHub App private key is not RSA")
	}
	return rsaKey, nil
}
