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

// Package lfs implements the Git LFS Batch and Verify HTTP protocol.
package lfs

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kode-blox/golfs/internal/authcache"
	"github.com/kode-blox/golfs/internal/config"
	"github.com/kode-blox/golfs/internal/forge"
	"github.com/kode-blox/golfs/internal/observability"
	"github.com/kode-blox/golfs/internal/storage"
)

// MediaType is the Git LFS JSON wire media type.
const MediaType = "application/vnd.git-lfs+json"

// Handler implements the Git LFS Batch and Verify protocol endpoints.
type Handler struct {
	publicURL     string
	maxObjectSize int64
	concurrency   int
	authorizer    forge.Authorizer
	cache         *authcache.Cache
	store         storage.ObjectStore
	metrics       *observability.Metrics
	logger        *slog.Logger
}

// Options supplies protocol limits and provider-independent dependencies.
type Options struct {
	PublicURL     string
	MaxObjectSize int64
	Concurrency   int
	Authorizer    forge.Authorizer
	Cache         *authcache.Cache
	Store         storage.ObjectStore
	Metrics       *observability.Metrics
	Logger        *slog.Logger
}

// New constructs a protocol handler from provider-independent dependencies.
func New(options Options) *Handler {
	concurrency := options.Concurrency
	if concurrency <= 0 {
		concurrency = config.ObjectCheckConcurrency
	}
	return &Handler{
		publicURL: strings.TrimRight(options.PublicURL, "/"), maxObjectSize: options.MaxObjectSize,
		concurrency: concurrency, authorizer: options.Authorizer, cache: options.Cache,
		store: options.Store, metrics: options.Metrics, logger: options.Logger,
	}
}

// Register adds all public GOLFS routes to mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /github.com/{owner}/{repository}/info/lfs/objects/batch", h.batch)
	mux.HandleFunc("POST /github.com/{owner}/{repository}/info/lfs/objects/verify", h.verify)
}

type batchRequest struct {
	Operation string          `json:"operation"`
	Transfers []string        `json:"transfers,omitempty"`
	Ref       json.RawMessage `json:"ref,omitempty"`
	HashAlgo  string          `json:"hash_algo,omitempty"`
	Objects   []requestObject `json:"objects"`
}

type requestObject struct {
	OID  string `json:"oid"`
	Size int64  `json:"size"`
}

type batchResponse struct {
	Transfer string           `json:"transfer"`
	HashAlgo string           `json:"hash_algo"`
	Objects  []responseObject `json:"objects"`
}

type responseObject struct {
	OID           string            `json:"oid"`
	Size          int64             `json:"size"`
	Authenticated bool              `json:"authenticated,omitempty"`
	Actions       map[string]action `json:"actions,omitempty"`
	Error         *objectError      `json:"error,omitempty"`
}

type action struct {
	Href          string            `json:"href"`
	Header        map[string]string `json:"header,omitempty"`
	ExpiresIn     int64             `json:"expires_in,omitempty"`
	Authenticated bool              `json:"authenticated,omitempty"`
}

type objectError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type errorResponse struct {
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

func (h *Handler) batch(w http.ResponseWriter, r *http.Request) {
	requestID := RequestID(r.Context())
	if !acceptsLFSJSON(r) {
		h.writeError(w, requestID, http.StatusNotAcceptable, "response media type is not acceptable")
		return
	}
	var input batchRequest
	if err := readJSON(w, r, &input); err != nil {
		status := http.StatusBadRequest
		if errors.As(err, new(*http.MaxBytesError)) {
			status = http.StatusRequestEntityTooLarge
		} else if errors.Is(err, errUnsupportedMediaType) {
			status = http.StatusUnsupportedMediaType
		}
		h.writeError(w, requestID, status, err.Error())
		return
	}
	if err := h.validateBatch(input); err != nil {
		h.writeError(w, requestID, http.StatusUnprocessableEntity, err.Error())
		return
	}
	token, originalAuthorization, ok := basicToken(r)
	if !ok {
		h.writeAuthenticationError(w, requestID, "Basic credentials are required")
		return
	}
	authorization, err := h.authorize(r.Context(), r.PathValue("owner"), r.PathValue("repository"), token)
	if err != nil {
		h.writeForgeError(w, requestID, err)
		return
	}
	if input.Operation == "download" && authorization.Permission < forge.PermissionRead {
		h.writeError(w, requestID, http.StatusNotFound, "repository not found")
		return
	}
	if input.Operation == "upload" && authorization.Permission < forge.PermissionWrite {
		h.writeError(w, requestID, http.StatusForbidden, "repository is read-only")
		return
	}

	output := batchResponse{Transfer: "basic", HashAlgo: "sha256", Objects: make([]responseObject, len(input.Objects))}
	h.processObjects(r.Context(), input.Operation, authorization.RepositoryID, r.PathValue("owner"), r.PathValue("repository"), originalAuthorization, input.Objects, output.Objects)
	h.writeJSON(w, http.StatusOK, output)
}

func (h *Handler) validateBatch(input batchRequest) error {
	if input.Operation != "upload" && input.Operation != "download" {
		return errors.New("operation must be upload or download")
	}
	if input.HashAlgo != "" && input.HashAlgo != "sha256" {
		return errors.New("hash_algo must be sha256")
	}
	if len(input.Transfers) > 0 {
		basic := false
		for _, transfer := range input.Transfers {
			if transfer == "basic" {
				basic = true
			}
		}
		if !basic {
			return errors.New("basic transfer is required")
		}
	}
	if len(input.Objects) > config.MaximumBatchObjects {
		return fmt.Errorf("batch contains more than %d objects", config.MaximumBatchObjects)
	}
	seen := make(map[string]int64, len(input.Objects))
	for _, object := range input.Objects {
		if err := h.validateObject(object); err != nil {
			return err
		}
		if previous, exists := seen[object.OID]; exists && previous != object.Size {
			return fmt.Errorf("OID %s has conflicting sizes", object.OID)
		}
		seen[object.OID] = object.Size
	}
	return nil
}

func (h *Handler) validateObject(object requestObject) error {
	if len(object.OID) != 64 {
		return errors.New("object OID must contain exactly 64 lowercase hexadecimal characters")
	}
	if _, err := hex.DecodeString(object.OID); err != nil || object.OID != strings.ToLower(object.OID) {
		return errors.New("object OID must contain exactly 64 lowercase hexadecimal characters")
	}
	if object.Size < 0 || object.Size > h.maxObjectSize {
		return fmt.Errorf("object size must be between 0 and %d", h.maxObjectSize)
	}
	return nil
}

func (h *Handler) processObjects(ctx context.Context, operation string, repositoryID int64, owner, repository, authorization string, input []requestObject, output []responseObject) {
	type job struct{ index int }
	jobs := make(chan job)
	var workers sync.WaitGroup
	workerCount := min(h.concurrency, max(1, len(input)))
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for item := range jobs {
				output[item.index] = h.processObject(ctx, operation, repositoryID, owner, repository, authorization, input[item.index])
			}
		}()
	}
	for index := range input {
		jobs <- job{index: index}
	}
	close(jobs)
	workers.Wait()
}

func (h *Handler) processObject(ctx context.Context, operation string, repositoryID int64, owner, repository, authorization string, input requestObject) responseObject {
	response := responseObject{OID: input.OID, Size: input.Size, Authenticated: true}
	stored, err := h.store.Head(ctx, repositoryID, input.OID)
	if operation == "download" {
		if errors.Is(err, storage.ErrNotFound) {
			return h.objectFailure(response, operation, http.StatusNotFound, "object not found", "not_found")
		}
		if err != nil {
			return h.objectFailure(response, operation, http.StatusServiceUnavailable, "object storage is temporarily unavailable", "unavailable")
		}
		if !matches(stored, input) {
			return h.objectFailure(response, operation, http.StatusUnprocessableEntity, "stored object failed integrity validation", "mismatch")
		}
		download, err := h.store.PresignDownload(ctx, repositoryID, input.OID)
		if err != nil {
			return h.objectFailure(response, operation, http.StatusServiceUnavailable, "object storage is temporarily unavailable", "unavailable")
		}
		response.Actions = map[string]action{"download": fromStorageAction(download, true)}
		h.observeObject(operation, "download")
		return response
	}

	if err == nil {
		if matches(stored, input) {
			h.observeObject(operation, "exists")
			return response
		}
		return h.objectFailure(response, operation, http.StatusUnprocessableEntity, "an object with this OID has mismatched storage metadata", "mismatch")
	}
	if !errors.Is(err, storage.ErrNotFound) {
		return h.objectFailure(response, operation, http.StatusServiceUnavailable, "object storage is temporarily unavailable", "unavailable")
	}
	upload, err := h.store.PresignUpload(ctx, repositoryID, input.OID, input.Size)
	if err != nil {
		return h.objectFailure(response, operation, http.StatusServiceUnavailable, "object storage is temporarily unavailable", "unavailable")
	}
	response.Actions = map[string]action{
		"upload": fromStorageAction(upload, false),
		"verify": {
			Href:   h.publicURL + "/github.com/" + url.PathEscape(owner) + "/" + url.PathEscape(repository) + "/info/lfs/objects/verify",
			Header: map[string]string{"Authorization": authorization}, Authenticated: true,
		},
	}
	h.observeObject(operation, "upload")
	return response
}

func (h *Handler) objectFailure(response responseObject, operation string, code int, message, result string) responseObject {
	response.Error = &objectError{Code: code, Message: message}
	h.observeObject(operation, result)
	return response
}

func (h *Handler) observeObject(operation, result string) {
	if h.metrics != nil {
		h.metrics.BatchObjects.WithLabelValues(operation, result).Inc()
	}
}

func matches(stored storage.Object, requested requestObject) bool {
	return stored.Size == requested.Size && stored.ChecksumSHA256 == oidChecksum(requested.OID)
}

func oidChecksum(oid string) string {
	digest, _ := hex.DecodeString(oid)
	return base64.StdEncoding.EncodeToString(digest)
}

func fromStorageAction(value storage.Action, authenticated bool) action {
	return action{Href: value.Href, Header: value.Header, ExpiresIn: value.ExpiresIn, Authenticated: authenticated}
}

func (h *Handler) verify(w http.ResponseWriter, r *http.Request) {
	requestID := RequestID(r.Context())
	if !acceptsLFSJSON(r) {
		h.writeError(w, requestID, http.StatusNotAcceptable, "response media type is not acceptable")
		return
	}
	var input requestObject
	if err := readJSON(w, r, &input); err != nil {
		status := http.StatusBadRequest
		if errors.As(err, new(*http.MaxBytesError)) {
			status = http.StatusRequestEntityTooLarge
		} else if errors.Is(err, errUnsupportedMediaType) {
			status = http.StatusUnsupportedMediaType
		}
		h.writeError(w, requestID, status, err.Error())
		return
	}
	if err := h.validateObject(input); err != nil {
		h.writeError(w, requestID, http.StatusUnprocessableEntity, err.Error())
		return
	}
	token, _, ok := basicToken(r)
	if !ok {
		h.writeAuthenticationError(w, requestID, "Basic credentials are required")
		return
	}
	authorization, err := h.authorize(r.Context(), r.PathValue("owner"), r.PathValue("repository"), token)
	if err != nil {
		h.writeForgeError(w, requestID, err)
		return
	}
	if authorization.Permission < forge.PermissionWrite {
		h.writeError(w, requestID, http.StatusForbidden, "repository is read-only")
		return
	}
	stored, err := h.store.Head(r.Context(), authorization.RepositoryID, input.OID)
	if errors.Is(err, storage.ErrNotFound) {
		h.observeVerify("not_found")
		h.writeError(w, requestID, http.StatusNotFound, "object not found")
		return
	}
	if err != nil {
		h.observeVerify("unavailable")
		h.writeError(w, requestID, http.StatusServiceUnavailable, "object storage is temporarily unavailable")
		return
	}
	if !matches(stored, input) {
		h.observeVerify("mismatch")
		h.writeError(w, requestID, http.StatusUnprocessableEntity, "uploaded object failed size or SHA-256 verification")
		return
	}
	h.observeVerify("success")
	h.writeJSON(w, http.StatusOK, map[string]string{"oid": input.OID})
}

func (h *Handler) observeVerify(result string) {
	if h.metrics != nil {
		h.metrics.VerifyResults.WithLabelValues(result).Inc()
	}
}

func (h *Handler) authorize(ctx context.Context, owner, repository, token string) (forge.Authorization, error) {
	if authorization, ok := h.cache.Get(token, owner, repository); ok {
		if h.metrics != nil {
			h.metrics.AuthCache.WithLabelValues("hit").Inc()
		}
		return authorization, nil
	}
	if h.metrics != nil {
		h.metrics.AuthCache.WithLabelValues("miss").Inc()
	}
	authorization, err := h.authorizer.Authorize(ctx, owner, repository, token)
	if err != nil {
		return forge.Authorization{}, err
	}
	h.cache.Put(token, owner, repository, authorization)
	return authorization, nil
}

func basicToken(r *http.Request) (token, original string, ok bool) {
	username, password, ok := r.BasicAuth()
	_ = username
	if !ok || password == "" {
		return "", "", false
	}
	return password, r.Header.Get("Authorization"), true
}

var errUnsupportedMediaType = errors.New("Content-Type must be application/vnd.git-lfs+json")

func readJSON(w http.ResponseWriter, r *http.Request, target any) error {
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]))
	if mediaType != MediaType && mediaType != "application/json" {
		return errUnsupportedMediaType
	}
	r.Body = http.MaxBytesReader(w, r.Body, config.MaximumBatchRequestSize)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode request: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err != nil {
			return fmt.Errorf("decode request: %w", err)
		}
		return errors.New("request body must contain one JSON value")
	}
	return nil
}

func acceptsLFSJSON(r *http.Request) bool {
	accept := strings.ToLower(r.Header.Get("Accept"))
	return accept == "" || strings.Contains(accept, "*/*") || strings.Contains(accept, MediaType) || strings.Contains(accept, "application/json")
}

func (h *Handler) writeForgeError(w http.ResponseWriter, requestID string, err error) {
	var retry *forge.RetryAfterError
	switch {
	case errors.As(err, &retry), errors.Is(err, forge.ErrRateLimited):
		delay := time.Minute
		if retry != nil && retry.RetryAfter > 0 {
			delay = retry.RetryAfter
		}
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(delay.Round(time.Second)/time.Second))))
		h.writeError(w, requestID, http.StatusTooManyRequests, "GitHub rate limit exceeded")
	case errors.Is(err, forge.ErrUnauthenticated):
		h.writeAuthenticationError(w, requestID, "credentials are invalid or expired")
	case errors.Is(err, forge.ErrNotFound), errors.Is(err, forge.ErrForbidden):
		h.writeError(w, requestID, http.StatusNotFound, "repository not found")
	default:
		h.writeError(w, requestID, http.StatusServiceUnavailable, "GitHub is temporarily unavailable")
	}
}

func (h *Handler) writeAuthenticationError(w http.ResponseWriter, requestID, message string) {
	w.Header().Set("LFS-Authenticate", `Basic realm="GOLFS"`)
	h.writeError(w, requestID, http.StatusUnauthorized, message)
}

func (h *Handler) writeError(w http.ResponseWriter, requestID string, status int, message string) {
	h.writeJSON(w, status, errorResponse{Message: message, RequestID: requestID})
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", MediaType)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil && h.logger != nil {
		h.logger.Error("write HTTP response", "error", err)
	}
}

type requestIDKey struct{}

// RequestID returns the server-generated request identifier stored in ctx.
func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey{}).(string)
	return value
}

// Middleware adds request IDs and redacting panic recovery.
func Middleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := newRequestID()
		w.Header().Set("X-Request-ID", requestID)
		ctx := context.WithValue(r.Context(), requestIDKey{}, requestID)
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("panic recovered", "request_id", requestID)
				w.Header().Set("Content-Type", MediaType)
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(errorResponse{Message: "internal server error", RequestID: requestID})
			}
		}()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func newRequestID() string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(value)
}
