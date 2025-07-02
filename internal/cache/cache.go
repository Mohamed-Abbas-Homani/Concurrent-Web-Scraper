package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
	"webscraper/pkg/types"

	"github.com/sirupsen/logrus"
)

// Cache provides thread-safe caching with TTL support
type Cache struct {
	data   map[string]*types.CacheEntry
	mutex  sync.RWMutex
	logger *logrus.Logger
	stats  CacheStats
}

// CacheStats holds cache performance statistics
type CacheStats struct {
	Hits         int64 `json:"hits"`
	Misses       int64 `json:"misses"`
	Evictions    int64 `json:"evictions"`
	TotalEntries int64 `json:"total_entries"`
}

// NewCache creates a new cache instance
func NewCache(logger *logrus.Logger) *Cache {
	cache := &Cache{
		data:   make(map[string]*types.CacheEntry),
		logger: logger,
		stats:  CacheStats{},
	}

	// Start cleanup goroutine
	go cache.cleanup()
	return cache
}

// Set stores a result in the cache with TTL
func (c *Cache) Set(key string, result *types.ScrapingResult, ttl time.Duration) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Create a copy to avoid mutation issues
	resultCopy := *result
	resultCopy.FromCache = true

	c.data[key] = &types.CacheEntry{
		Data:      &resultCopy,
		ExpiresAt: time.Now().Add(ttl),
	}

	c.stats.TotalEntries++
	c.logger.WithFields(logrus.Fields{
		"key": key,
		"ttl": ttl,
	}).Debug("Cache entry set")
}

// Get retrieves a result from the cache
func (c *Cache) Get(key string) (*types.ScrapingResult, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	entry, exists := c.data[key]
	if !exists {
		c.stats.Misses++
		return nil, false
	}

	if time.Now().After(entry.ExpiresAt) {
		c.stats.Misses++
		c.logger.WithField("key", key).Debug("Cache entry expired")
		return nil, false
	}

	c.stats.Hits++
	c.logger.WithField("key", key).Debug("Cache hit")
	return entry.Data, true
}

// Delete removes an entry from the cache
func (c *Cache) Delete(key string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if _, exists := c.data[key]; exists {
		delete(c.data, key)
		c.stats.Evictions++
		c.logger.WithField("key", key).Debug("Cache entry deleted")
	}
}

// Clear removes all entries from the cache
func (c *Cache) Clear() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	count := len(c.data)
	c.data = make(map[string]*types.CacheEntry)
	c.stats.Evictions += int64(count)
	c.logger.WithField("count", count).Info("Cache cleared")
}

// GetStats returns cache statistics
func (c *Cache) GetStats() CacheStats {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	stats := c.stats
	stats.TotalEntries = int64(len(c.data))
	return stats
}

// GenerateKey creates a cache key from task parameters
func (c *Cache) GenerateKey(task *types.ScrapingTask) string {
	data, _ := json.Marshal(map[string]interface{}{
		"url":       task.URL,
		"selectors": task.Selectors,
		"method":    task.Method,
		"headers":   task.Headers,
	})

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// cleanup removes expired entries periodically
func (c *Cache) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.mutex.Lock()
		now := time.Now()
		expired := 0

		for key, entry := range c.data {
			if now.After(entry.ExpiresAt) {
				delete(c.data, key)
				expired++
			}
		}

		if expired > 0 {
			c.stats.Evictions += int64(expired)
			c.logger.WithField("expired_count", expired).Debug("Cache cleanup completed")
		}
		c.mutex.Unlock()
	}
}
