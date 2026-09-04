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

// Package storage defines the provider-independent immutable object boundary.
package storage

import (
	"context"
	"errors"
)

// Provider-neutral object errors used by protocol error mapping.
var (
	ErrNotFound    = errors.New("storage: object not found")
	ErrUnavailable = errors.New("storage: unavailable")
)

// Object contains integrity metadata returned by the provider.
type Object struct {
	Size           int64
	ChecksumSHA256 string
}

// Action is a presigned direct-transfer request returned to a Git LFS client.
type Action struct {
	Href      string
	Header    map[string]string
	ExpiresIn int64
}

// ObjectStore is the storage boundary consumed by the LFS protocol. The
// repository ID is always part of object identity.
type ObjectStore interface {
	Head(ctx context.Context, repositoryID int64, oid string) (Object, error)
	PresignUpload(ctx context.Context, repositoryID int64, oid string, size int64) (Action, error)
	PresignDownload(ctx context.Context, repositoryID int64, oid string) (Action, error)
	Validate(ctx context.Context) error
}
