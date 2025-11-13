package cache

import (
	"time"

	gocache "github.com/patrickmn/go-cache"
)

// Cache defines the interface for caching operations
type Cache interface {
	Get(key string) (any, bool)
	Set(key string, value any, ttl time.Duration)
	Delete(key string)
	Clear()
}

// TTLCache implements Cache interface with time-to-live support
type TTLCache struct {
	data *gocache.Cache
}

// New creates a new TTL cache with default cleanup interval
func New(defaultTTL time.Duration) *TTLCache {
	cleanupInterval := defaultTTL * 2
	return &TTLCache{
		data: gocache.New(defaultTTL, cleanupInterval),
	}
}

// Get retrieves a value from the cache
func (c *TTLCache) Get(key string) (any, bool) {
	return c.data.Get(key)
}

// Set stores a value in the cache with the specified TTL
func (c *TTLCache) Set(key string, value any, ttl time.Duration) {
	c.data.Set(key, value, ttl)
}

// Delete removes a value from the cache
func (c *TTLCache) Delete(key string) {
	c.data.Delete(key)
}

// Clear removes all values from the cache
func (c *TTLCache) Clear() {
	c.data.Flush()
}
