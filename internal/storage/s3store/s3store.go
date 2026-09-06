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

// Package s3store implements immutable S3-backed object storage.
package s3store

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/kode-blox/golfs/internal/observability"
	"github.com/kode-blox/golfs/internal/storage"
)

// Config supplies the S3 endpoint, bucket, signing behavior, and metrics.
type Config struct {
	Endpoint     string
	Region       string
	Bucket       string
	UsePathStyle bool
	PresignTTL   time.Duration
	Metrics      *observability.Metrics
}

// Store implements immutable direct-transfer storage with AWS SDK for Go v2.
type Store struct {
	bucket    string
	ttl       time.Duration
	client    *s3.Client
	presigner *s3.PresignClient
	metrics   *observability.Metrics
}

// New loads the standard AWS credential chain and constructs an S3 store.
func New(ctx context.Context, cfg Config) (*Store, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.Region))
	if err != nil {
		return nil, fmt.Errorf("load AWS configuration: %w", err)
	}
	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(cfg.Endpoint)
		options.UsePathStyle = cfg.UsePathStyle
	})
	return newWithClient(client, cfg), nil
}

func newWithClient(client *s3.Client, cfg Config) *Store {
	return &Store{
		bucket: cfg.Bucket, ttl: cfg.PresignTTL, client: client,
		presigner: s3.NewPresignClient(client), metrics: cfg.Metrics,
	}
}

// Validate checks that the configured bucket is reachable during startup.
func (s *Store) Validate(ctx context.Context) error {
	started := time.Now()
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(s.bucket)})
	s.observe("head_bucket", result(err), started)
	if err != nil {
		return fmt.Errorf("check S3 bucket access: %w", mapError(err))
	}
	return nil
}

// Head returns the size of an object accepted by the storage provider.
func (s *Store) Head(ctx context.Context, repositoryID int64, oid string) (storage.Object, error) {
	key, err := Key(repositoryID, oid)
	if err != nil {
		return storage.Object{}, err
	}
	started := time.Now()
	output, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
	})
	s.observe("head_object", result(err), started)
	if err != nil {
		return storage.Object{}, mapError(err)
	}
	return storage.Object{Size: aws.ToInt64(output.ContentLength)}, nil
}

// PresignUpload creates a conditional, payload-hash-bound, single-part PUT action.
func (s *Store) PresignUpload(ctx context.Context, repositoryID int64, oid string, size int64) (storage.Action, error) {
	key, err := Key(repositoryID, oid)
	if err != nil {
		return storage.Action{}, err
	}
	started := time.Now()
	request, err := s.presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
		ContentLength: aws.Int64(size),
		IfNoneMatch:   aws.String("*"),
	}, func(options *s3.PresignOptions) {
		options.Expires = s.ttl
		options.ClientOptions = append(options.ClientOptions, func(clientOptions *s3.Options) {
			clientOptions.APIOptions = append(clientOptions.APIOptions, bindPayloadHash(oid))
		})
	})
	s.observe("presign_put_object", result(err), started)
	if err != nil {
		return storage.Action{}, fmt.Errorf("presign S3 upload: %w", storage.ErrUnavailable)
	}
	return actionFromRequest(request.URL, request.SignedHeader, s.ttl), nil
}

// PresignDownload creates a direct GET action for a previously validated object.
func (s *Store) PresignDownload(ctx context.Context, repositoryID int64, oid string) (storage.Action, error) {
	key, err := Key(repositoryID, oid)
	if err != nil {
		return storage.Action{}, err
	}
	started := time.Now()
	request, err := s.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
	}, func(options *s3.PresignOptions) { options.Expires = s.ttl })
	s.observe("presign_get_object", result(err), started)
	if err != nil {
		return storage.Action{}, fmt.Errorf("presign S3 download: %w", storage.ErrUnavailable)
	}
	return actionFromRequest(request.URL, request.SignedHeader, s.ttl), nil
}

// Key returns the repository-scoped, Git LFS-style sharded object key.
func Key(repositoryID int64, oid string) (string, error) {
	if len(oid) != 64 || oid != strings.ToLower(oid) {
		return "", errors.New("invalid SHA-256 OID")
	}
	if digest, err := hex.DecodeString(oid); err != nil || len(digest) != 32 {
		return "", errors.New("invalid SHA-256 OID")
	}
	return "github/" + strconv.FormatInt(repositoryID, 10) + "/objects/" + oid[:2] + "/" + oid[2:4] + "/" + oid, nil
}

// staticPayloadHash supplies the already validated Git LFS OID to the SDK's
// standard SigV4 presigner without reading the client-owned request body.
type staticPayloadHash struct {
	value string
}

func (*staticPayloadHash) ID() string { return (&v4.UnsignedPayload{}).ID() }

func (m *staticPayloadHash) HandleFinalize(
	ctx context.Context,
	in middleware.FinalizeInput,
	next middleware.FinalizeHandler,
) (middleware.FinalizeOutput, middleware.Metadata, error) {
	return next.HandleFinalize(v4.SetPayloadHash(ctx, m.value), in)
}

func bindPayloadHash(payloadHash string) func(*middleware.Stack) error {
	return func(stack *middleware.Stack) error {
		if _, err := stack.Finalize.Swap((&v4.UnsignedPayload{}).ID(), &staticPayloadHash{value: payloadHash}); err != nil {
			return fmt.Errorf("replace S3 presign payload hash middleware: %w", err)
		}
		if err := v4.AddContentSHA256HeaderMiddleware(stack); err != nil {
			return fmt.Errorf("add S3 content SHA-256 header middleware: %w", err)
		}
		return nil
	}
}

func actionFromRequest(href string, signed http.Header, ttl time.Duration) storage.Action {
	headers := make(map[string]string, len(signed))
	for name, values := range signed {
		if len(values) > 0 && !strings.EqualFold(name, "host") {
			headers[name] = values[0]
		}
	}
	return storage.Action{Href: href, Header: headers, ExpiresIn: int64(ttl.Seconds())}
}

func mapError(err error) error {
	var responseError *smithyhttp.ResponseError
	if errors.As(err, &responseError) {
		switch responseError.HTTPStatusCode() {
		case http.StatusNotFound:
			return storage.ErrNotFound
		case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return storage.ErrUnavailable
		}
	}
	var apiError smithy.APIError
	if errors.As(err, &apiError) {
		switch apiError.ErrorCode() {
		case "NotFound", "NoSuchKey", "NoSuchBucket":
			return storage.ErrNotFound
		case "SlowDown", "RequestTimeout", "ServiceUnavailable", "InternalError":
			return storage.ErrUnavailable
		}
	}
	return storage.ErrUnavailable
}

func result(err error) string {
	if err == nil {
		return "success"
	}
	if errors.Is(mapError(err), storage.ErrNotFound) {
		return "not_found"
	}
	return "error"
}

func (s *Store) observe(operation, status string, started time.Time) {
	if s.metrics == nil {
		return
	}
	s.metrics.S3Requests.WithLabelValues(operation, status).Inc()
	s.metrics.S3Duration.WithLabelValues(operation).Observe(time.Since(started).Seconds())
}
