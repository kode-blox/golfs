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

// Package forge defines the provider-independent repository authorization boundary.
package forge

import (
	"context"
	"errors"
	"time"
)

// Provider-neutral authorization errors used by protocol error mapping.
var (
	ErrUnauthenticated = errors.New("forge: unauthenticated")
	ErrNotFound        = errors.New("forge: repository not found")
	ErrForbidden       = errors.New("forge: forbidden")
	ErrRateLimited     = errors.New("forge: rate limited")
	ErrUnavailable     = errors.New("forge: unavailable")
)

// Permission is the effective repository access granted to a user token.
type Permission uint8

// Supported effective repository permissions.
const (
	PermissionNone Permission = iota
	PermissionRead
	PermissionWrite
)

// Authorization contains a stable repository identity and effective permission.
type Authorization struct {
	RepositoryID  int64
	Permission    Permission
	CanonicalName string
}

// Authorizer resolves a user token and repository path into a stable identity
// and effective permission. Implementations must fail closed.
type Authorizer interface {
	Authorize(ctx context.Context, owner, repository, token string) (Authorization, error)
}

// Validator performs startup validation that cannot be covered by parsing
// configuration alone.
type Validator interface {
	Validate(ctx context.Context) error
}

// RetryAfterError carries the server-supplied delay for a rate-limit response.
type RetryAfterError struct {
	Err        error
	RetryAfter time.Duration
}

func (e *RetryAfterError) Error() string { return e.Err.Error() }
func (e *RetryAfterError) Unwrap() error { return e.Err }
