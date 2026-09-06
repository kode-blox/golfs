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

package s3store

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const testOID = "dff443a64ae9388d167d9f308ef1df830ce25917ac98a3c19d5cd7a18e250813"

func TestKey(t *testing.T) {
	t.Parallel()

	key, err := Key(1354285337, testOID)
	if err != nil {
		t.Fatalf("Key returned an unexpected error: %v", err)
	}
	want := "github/1354285337/objects/df/f4/" + testOID
	if key != want {
		t.Fatalf("Key = %q, want %q", key, want)
	}
}

func TestKeyRejectsInvalidOID(t *testing.T) {
	t.Parallel()

	for _, oid := range []string{
		"short",
		strings.ToUpper(testOID),
		strings.Repeat("z", 64),
	} {
		oid := oid
		t.Run(oid[:min(8, len(oid))], func(t *testing.T) {
			t.Parallel()
			if _, err := Key(1354285337, oid); err == nil {
				t.Fatalf("Key accepted invalid OID %q", oid)
			}
		})
	}
}

func TestPresignUploadBindsContentSHA256(t *testing.T) {
	t.Parallel()

	store := newTestStore("https://objects.example.test")
	action, err := store.PresignUpload(context.Background(), 1354285337, testOID, 1174)
	if err != nil {
		t.Fatalf("PresignUpload returned an unexpected error: %v", err)
	}

	assertActionHeader(t, action.Header, "X-Amz-Content-Sha256", testOID)
	assertActionHeader(t, action.Header, "Content-Length", "1174")
	assertActionHeader(t, action.Header, "If-None-Match", "*")
	if value := actionHeader(action.Header, "X-Amz-Checksum-Sha256"); value != "" {
		t.Fatalf("X-Amz-Checksum-Sha256 = %q, want header to be absent", value)
	}

	parsed, err := url.Parse(action.Href)
	if err != nil {
		t.Fatalf("parse presigned URL: %v", err)
	}
	wantPath := "/golfs-test/github/1354285337/objects/df/f4/" + testOID
	if parsed.Path != wantPath {
		t.Fatalf("presigned path = %q, want %q", parsed.Path, wantPath)
	}
	signedHeaders := strings.Split(parsed.Query().Get("X-Amz-SignedHeaders"), ";")
	for _, name := range []string{"content-length", "host", "if-none-match", "x-amz-content-sha256"} {
		if !contains(signedHeaders, name) {
			t.Errorf("signed headers %q do not contain %q", signedHeaders, name)
		}
	}
	if contains(signedHeaders, "x-amz-checksum-sha256") {
		t.Errorf("signed headers %q unexpectedly contain x-amz-checksum-sha256", signedHeaders)
	}
	if action.ExpiresIn != int64(time.Hour/time.Second) {
		t.Fatalf("ExpiresIn = %d, want %d", action.ExpiresIn, int64(time.Hour/time.Second))
	}
}

func TestPresignUploadRejectsInvalidOID(t *testing.T) {
	t.Parallel()

	store := newTestStore("https://objects.example.test")
	if _, err := store.PresignUpload(context.Background(), 1, "short", 1); err == nil {
		t.Fatal("PresignUpload accepted an invalid OID")
	}
}

func TestHeadReturnsSizeWithoutChecksumMetadata(t *testing.T) {
	t.Parallel()

	var requestedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		w.Header().Set("Content-Length", "1174")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	store := newTestStore(server.URL)
	object, err := store.Head(context.Background(), 1354285337, testOID)
	if err != nil {
		t.Fatalf("Head returned an unexpected error: %v", err)
	}
	if object.Size != 1174 {
		t.Fatalf("Head size = %d, want 1174", object.Size)
	}
	wantPath := "/golfs-test/github/1354285337/objects/df/f4/" + testOID
	if requestedPath != wantPath {
		t.Fatalf("Head path = %q, want %q", requestedPath, wantPath)
	}
}

func newTestStore(endpoint string) *Store {
	client := s3.NewFromConfig(aws.Config{
		Region:      "fsn1",
		Credentials: credentials.NewStaticCredentialsProvider("test-access-key", "test-secret-key", ""),
	}, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(endpoint)
		options.UsePathStyle = true
	})
	return newWithClient(client, Config{Bucket: "golfs-test", PresignTTL: time.Hour})
}

func assertActionHeader(t *testing.T, headers map[string]string, name, want string) {
	t.Helper()
	if got := actionHeader(headers, name); got != want {
		t.Fatalf("%s = %q, want %q; all headers: %v", name, got, want, headers)
	}
}

func actionHeader(headers map[string]string, name string) string {
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
