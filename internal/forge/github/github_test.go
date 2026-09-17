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

package github

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/kode-blox/golfs/internal/forge"
)

const testClientID = "Iv1_test"

func TestAuthorizeAcceptsAllowlistedInstallation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.RequestURI() {
		case "/repos/greybodygames/andromeda/installation":
			writeJSON(t, w, `{"id":42,"client_id":"Iv1_test"}`)
		case "/user/installations?per_page=100":
			if got := r.Header.Get("Authorization"); got != "Bearer ghu_test" {
				t.Errorf("user installations Authorization = %q", got)
			}
			writeJSON(t, w, `{"installations":[{"id":42}]}`)
		case "/repos/greybodygames/andromeda":
			if got := r.Header.Get("Authorization"); got != "Bearer ghu_test" {
				t.Errorf("repository Authorization = %q", got)
			}
			writeJSON(t, w, `{"id":99,"full_name":"greybodygames/andromeda","permissions":{"pull":true,"push":true}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	authorizer := newTestAuthorizer(t, server, []int64{42})
	got, err := authorizer.Authorize(context.Background(), "greybodygames", "andromeda", "ghu_test")
	if err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}
	if got.RepositoryID != 99 || got.CanonicalName != "greybodygames/andromeda" || got.Permission != forge.PermissionWrite {
		t.Fatalf("Authorize() = %+v", got)
	}
}

func TestAuthorizeRejectsInstallationOutsideAllowlistBeforeUserLookup(t *testing.T) {
	t.Parallel()

	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.RequestURI() != "/repos/outsider/repository/installation" {
			t.Errorf("unexpected request %s", r.URL.RequestURI())
		}
		writeJSON(t, w, `{"id":77,"client_id":"Iv1_test"}`)
	}))
	defer server.Close()

	authorizer := newTestAuthorizer(t, server, []int64{42})
	_, err := authorizer.Authorize(context.Background(), "outsider", "repository", "ghu_test")
	if !errors.Is(err, forge.ErrNotFound) {
		t.Fatalf("Authorize() error = %v, want %v", err, forge.ErrNotFound)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("Authorize() made %d requests, want 1", got)
	}
}

func TestValidateChecksEveryAllowlistedInstallation(t *testing.T) {
	t.Parallel()

	seen := make(map[string]bool)
	var seenMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenMu.Lock()
		seen[r.URL.Path] = true
		seenMu.Unlock()
		switch r.URL.Path {
		case "/app":
			writeJSON(t, w, `{"client_id":"Iv1_test"}`)
		case "/app/installations/42":
			writeJSON(t, w, `{"id":42,"client_id":"Iv1_test"}`)
		case "/app/installations/84":
			writeJSON(t, w, `{"id":84,"client_id":"Iv1_test"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	authorizer := newTestAuthorizer(t, server, []int64{42, 84})
	if err := authorizer.Validate(context.Background()); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	seenMu.Lock()
	defer seenMu.Unlock()
	for _, path := range []string{"/app", "/app/installations/42", "/app/installations/84"} {
		if !seen[path] {
			t.Errorf("Validate() did not request %s", path)
		}
	}
}

func TestNewRequiresAllowedInstallationIDs(t *testing.T) {
	t.Parallel()

	_, err := New(Config{ClientID: testClientID, PrivateKeyPEM: testPrivateKeyPEM(t)})
	if err == nil || !strings.Contains(err.Error(), "at least one allowed") {
		t.Fatalf("New() error = %v", err)
	}
}

func newTestAuthorizer(t *testing.T, server *httptest.Server, allowed []int64) *Authorizer {
	t.Helper()
	authorizer, err := New(Config{
		ClientID:               testClientID,
		PrivateKeyPEM:          testPrivateKeyPEM(t),
		AllowedInstallationIDs: allowed,
		APIURL:                 server.URL,
		HTTPClient:             server.Client(),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return authorizer
}

func testPrivateKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
}

func writeJSON(t *testing.T, w http.ResponseWriter, value string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write([]byte(value)); err != nil {
		t.Errorf("write response: %v", err)
	}
}
