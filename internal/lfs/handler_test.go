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

package lfs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kode-blox/golfs/internal/authcache"
	"github.com/kode-blox/golfs/internal/forge"
	"github.com/kode-blox/golfs/internal/observability"
	"github.com/kode-blox/golfs/internal/storage"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

const handlerTestOID = "dff443a64ae9388d167d9f308ef1df830ce25917ac98a3c19d5cd7a18e250813"

func TestBatchUploadObjectStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		stored     storage.Object
		headErr    error
		wantResult string
		wantAction bool
		wantCode   int
	}{
		{name: "absent", headErr: storage.ErrNotFound, wantResult: "upload", wantAction: true},
		{name: "same size", stored: storage.Object{Size: 1174}, wantResult: "exists"},
		{name: "different size", stored: storage.Object{Size: 1173}, wantResult: "mismatch", wantCode: http.StatusUnprocessableEntity},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := &fakeStore{
				stored:  tt.stored,
				headErr: tt.headErr,
				upload: storage.Action{
					Href:      "https://objects.example.test/upload",
					Header:    map[string]string{"X-Amz-Content-Sha256": handlerTestOID},
					ExpiresIn: 3600,
				},
			}
			mux, metrics := newHandlerTestServer(t, store)
			response := performLFSRequest(t, mux, batchPath, fmt.Sprintf(
				`{"operation":"upload","objects":[{"oid":%q,"size":1174}]}`,
				handlerTestOID,
			))
			if response.Code != http.StatusOK {
				t.Fatalf("Batch status = %d, want 200; body: %s", response.Code, response.Body.String())
			}

			var output batchResponse
			decodeResponse(t, response, &output)
			if len(output.Objects) != 1 {
				t.Fatalf("Batch returned %d objects, want 1", len(output.Objects))
			}
			object := output.Objects[0]
			if tt.wantCode != 0 {
				if object.Error == nil || object.Error.Code != tt.wantCode {
					t.Fatalf("object error = %#v, want code %d", object.Error, tt.wantCode)
				}
			} else if object.Error != nil {
				t.Fatalf("object error = %#v, want none", object.Error)
			}
			if tt.wantAction {
				upload, ok := object.Actions["upload"]
				if !ok {
					t.Fatalf("upload action missing from %#v", object.Actions)
				}
				if upload.Header["X-Amz-Content-Sha256"] != handlerTestOID {
					t.Fatalf("upload header = %#v, want content SHA-256", upload.Header)
				}
				if _, ok := object.Actions["verify"]; !ok {
					t.Fatalf("verify action missing from %#v", object.Actions)
				}
			} else if len(object.Actions) != 0 {
				t.Fatalf("actions = %#v, want none", object.Actions)
			}
			assertCounter(t, metrics.BatchObjects.WithLabelValues("upload", tt.wantResult), 1)
		})
	}
}

func TestVerifyOutcomes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		stored     storage.Object
		headErr    error
		wantStatus int
		wantResult string
	}{
		{name: "success", stored: storage.Object{Size: 1174}, wantStatus: http.StatusOK, wantResult: "success"},
		{name: "not found", headErr: storage.ErrNotFound, wantStatus: http.StatusNotFound, wantResult: "not_found"},
		{name: "different size", stored: storage.Object{Size: 1173}, wantStatus: http.StatusUnprocessableEntity, wantResult: "mismatch"},
		{name: "unavailable", headErr: storage.ErrUnavailable, wantStatus: http.StatusServiceUnavailable, wantResult: "unavailable"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := &fakeStore{stored: tt.stored, headErr: tt.headErr}
			mux, metrics := newHandlerTestServer(t, store)
			response := performLFSRequest(t, mux, verifyPath, fmt.Sprintf(
				`{"oid":%q,"size":1174}`,
				handlerTestOID,
			))
			if response.Code != tt.wantStatus {
				t.Fatalf("Verify status = %d, want %d; body: %s", response.Code, tt.wantStatus, response.Body.String())
			}
			assertCounter(t, metrics.VerifyResults.WithLabelValues(tt.wantResult), 1)
		})
	}
}

func TestBatchDownloadUsesSizeValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		storedSize int64
		wantCode   int
		wantAction bool
	}{
		{name: "same size", storedSize: 1174, wantAction: true},
		{name: "different size", storedSize: 1173, wantCode: http.StatusUnprocessableEntity},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			store := &fakeStore{
				stored: storage.Object{Size: tt.storedSize},
				download: storage.Action{
					Href:      "https://objects.example.test/download",
					ExpiresIn: 3600,
				},
			}
			mux, metrics := newHandlerTestServer(t, store)
			response := performLFSRequest(t, mux, batchPath, fmt.Sprintf(
				`{"operation":"download","objects":[{"oid":%q,"size":1174}]}`,
				handlerTestOID,
			))
			if response.Code != http.StatusOK {
				t.Fatalf("Batch status = %d, want 200; body: %s", response.Code, response.Body.String())
			}
			var output batchResponse
			decodeResponse(t, response, &output)
			object := output.Objects[0]
			if tt.wantCode != 0 {
				if object.Error == nil || object.Error.Code != tt.wantCode {
					t.Fatalf("object error = %#v, want code %d", object.Error, tt.wantCode)
				}
			} else if object.Error != nil {
				t.Fatalf("object error = %#v, want none", object.Error)
			}
			_, hasDownload := object.Actions["download"]
			if hasDownload != tt.wantAction {
				t.Fatalf("download action present = %t, want %t", hasDownload, tt.wantAction)
			}
			result := "mismatch"
			if tt.wantAction {
				result = "download"
			}
			assertCounter(t, metrics.BatchObjects.WithLabelValues("download", result), 1)
		})
	}
}

const (
	batchPath  = "/github.com/greybodygames/andromeda/info/lfs/objects/batch"
	verifyPath = "/github.com/greybodygames/andromeda/info/lfs/objects/verify"
)

type fakeAuthorizer struct{}

func (fakeAuthorizer) Authorize(context.Context, string, string, string) (forge.Authorization, error) {
	return forge.Authorization{RepositoryID: 1354285337, Permission: forge.PermissionWrite}, nil
}

type fakeStore struct {
	stored   storage.Object
	headErr  error
	upload   storage.Action
	download storage.Action
}

func (f *fakeStore) Head(context.Context, int64, string) (storage.Object, error) {
	return f.stored, f.headErr
}

func (f *fakeStore) PresignUpload(context.Context, int64, string, int64) (storage.Action, error) {
	return f.upload, nil
}

func (f *fakeStore) PresignDownload(context.Context, int64, string) (storage.Action, error) {
	return f.download, nil
}

func (*fakeStore) Validate(context.Context) error { return nil }

func newHandlerTestServer(t *testing.T, store storage.ObjectStore) (*http.ServeMux, *observability.Metrics) {
	t.Helper()
	cache, err := authcache.New(8, time.Minute)
	if err != nil {
		t.Fatalf("create authorization cache: %v", err)
	}
	metrics := observability.New("test", "test")
	handler := New(Options{
		PublicURL:     "https://lfs.example.test",
		MaxObjectSize: 5_000_000_000,
		Concurrency:   1,
		Authorizer:    fakeAuthorizer{},
		Cache:         cache,
		Store:         store,
		Metrics:       metrics,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	mux := http.NewServeMux()
	handler.Register(mux)
	return mux, metrics
}

func performLFSRequest(t *testing.T, handler http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Accept", MediaType)
	request.Header.Set("Content-Type", MediaType)
	request.SetBasicAuth("git", "ghu_test")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeResponse(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func assertCounter(t *testing.T, counter prometheus.Counter, want float64) {
	t.Helper()
	var metric dto.Metric
	if err := counter.Write(&metric); err != nil {
		t.Fatalf("read counter: %v", err)
	}
	if got := metric.GetCounter().GetValue(); got != want {
		t.Fatalf("counter = %v, want %v", got, want)
	}
}

var _ forge.Authorizer = fakeAuthorizer{}
var _ storage.ObjectStore = (*fakeStore)(nil)
