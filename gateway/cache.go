package main

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"snaptry-gateway/provider"
)

// Cache deduplicates identical try-on requests so the user is not billed
// twice for the same person+garment pair. In-memory with TTL; a restart
// clears it (documented in DEPLOY.md).
type cacheEntry struct {
	url       string
	expiresAt time.Time
}

type Cache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	ttl     time.Duration
}

func NewCache(ttlHours int) *Cache {
	c := &Cache{entries: make(map[string]cacheEntry), ttl: time.Duration(ttlHours) * time.Hour}
	go c.janitor()
	return c
}

func cacheKey(req provider.TryOnRequest, providerName string) string {
	h := sha256.New()
	h.Write([]byte(providerName + "\x00" + req.Category + "\x00"))
	h.Write([]byte(req.PersonImage))
	h.Write([]byte("\x00"))
	h.Write([]byte(req.GarmentImage))
	return hex.EncodeToString(h.Sum(nil))
}

func (c *Cache) Get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[key]
	if !ok || time.Now().After(e.expiresAt) {
		return "", false
	}
	return e.url, true
}

func (c *Cache) Set(key, url string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = cacheEntry{url: url, expiresAt: time.Now().Add(c.ttl)}
}

func (c *Cache) janitor() {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for range t.C {
		now := time.Now()
		c.mu.Lock()
		for k, e := range c.entries {
			if now.After(e.expiresAt) {
				delete(c.entries, k)
			}
		}
		c.mu.Unlock()
	}
}
