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

// Package authcache provides bounded process-local authorization caching.
package authcache

import (
	"container/list"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kode-blox/golfs/internal/forge"
)

type clock func() time.Time

type entry struct {
	key       string
	value     forge.Authorization
	expiresAt time.Time
}

// Cache is a bounded, process-local LRU cache. Keys are HMACs so neither the
// raw bearer token nor a directly reusable token hash is retained in memory.
type Cache struct {
	mu       sync.Mutex
	secret   []byte
	ttl      time.Duration
	capacity int
	clock    clock
	items    map[string]*list.Element
	lru      *list.List
}

// New constructs a cache with a random per-process HMAC key.
func New(capacity int, ttl time.Duration) (*Cache, error) {
	if capacity <= 0 || ttl <= 0 {
		return nil, fmt.Errorf("cache capacity and TTL must be positive")
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("generate authorization cache secret: %w", err)
	}
	return newWithClock(capacity, ttl, secret, time.Now), nil
}

func newWithClock(capacity int, ttl time.Duration, secret []byte, now clock) *Cache {
	return &Cache{
		secret: append([]byte(nil), secret...), ttl: ttl, capacity: capacity,
		clock: now, items: make(map[string]*list.Element), lru: list.New(),
	}
}

// Get returns an unexpired authorization and updates its recency.
func (c *Cache) Get(token, owner, repository string) (forge.Authorization, bool) {
	key := c.key(token, owner, repository)
	c.mu.Lock()
	defer c.mu.Unlock()

	element, ok := c.items[key]
	if !ok {
		return forge.Authorization{}, false
	}
	item := element.Value.(*entry)
	if !c.clock().Before(item.expiresAt) {
		c.remove(element)
		return forge.Authorization{}, false
	}
	c.lru.MoveToFront(element)
	return item.value, true
}

// Put stores a positive authorization and evicts the least-recently-used item when full.
func (c *Cache) Put(token, owner, repository string, value forge.Authorization) {
	key := c.key(token, owner, repository)
	c.mu.Lock()
	defer c.mu.Unlock()

	if existing, ok := c.items[key]; ok {
		item := existing.Value.(*entry)
		item.value = value
		item.expiresAt = c.clock().Add(c.ttl)
		c.lru.MoveToFront(existing)
		return
	}
	element := c.lru.PushFront(&entry{key: key, value: value, expiresAt: c.clock().Add(c.ttl)})
	c.items[key] = element
	if c.lru.Len() > c.capacity {
		c.remove(c.lru.Back())
	}
}

func (c *Cache) key(token, owner, repository string) string {
	mac := hmac.New(sha256.New, c.secret)
	_, _ = mac.Write([]byte(token))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(strings.ToLower(owner)))
	_, _ = mac.Write([]byte{'/'})
	_, _ = mac.Write([]byte(strings.ToLower(repository)))
	return hex.EncodeToString(mac.Sum(nil))
}

func (c *Cache) remove(element *list.Element) {
	if element == nil {
		return
	}
	delete(c.items, element.Value.(*entry).key)
	c.lru.Remove(element)
}
