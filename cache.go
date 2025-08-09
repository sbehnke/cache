// A mostly vibe coded cache to learn about publishing go modules.
package cache

import (
	"container/list"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// CacheStats holds performance metrics for the cache
type CacheStats struct {
	Hits        int64 `json:"hits"`
	Misses      int64 `json:"misses"`
	Sets        int64 `json:"sets"`
	Evictions   int64 `json:"evictions"`
	Expirations int64 `json:"expirations"`
	Deletes     int64 `json:"deletes"`
}

// CacheCallbacks defines event handlers for cache operations
type CacheCallbacks struct {
	OnExpire func(key string, value any)
	OnEvict  func(key string, value any)
	OnSet    func(key string, value any)
	OnDelete func(key string, value any)
}

// Cache represents a thread-safe in-memory cache with advanced features
type Cache struct {
	mu          sync.RWMutex
	store       map[string]*cacheItem
	lruList     *list.List
	maxSize     int
	memoryLimit int64
	currentSize int64
	stats       CacheStats
	callbacks   *CacheCallbacks
	namespaces  map[string]map[string]*cacheItem
}

type cacheItem struct {
	key         string
	value       any
	expiration  time.Time
	element     *list.Element
	namespace   string
	originalTTL time.Duration
	size        int64
}

// SerializableItem represents a cache item for JSON serialization
type SerializableItem struct {
	Key         string        `json:"key"`
	Value       any           `json:"value"`
	Expiration  time.Time     `json:"expiration"`
	Namespace   string        `json:"namespace"`
	OriginalTTL time.Duration `json:"original_ttl"`
}

// CacheNamespace provides namespace-scoped operations
type CacheNamespace struct {
	cache     *Cache
	namespace string
}

// NewCache creates a new cache instance
func NewCache() *Cache {
	return &Cache{
		store:      make(map[string]*cacheItem),
		lruList:    list.New(),
		namespaces: make(map[string]map[string]*cacheItem),
	}
}

// NewCacheWithOptions creates a cache with specified options
func NewCacheWithOptions(maxSize int, memoryLimit int64, callbacks *CacheCallbacks) *Cache {
	c := NewCache()
	c.maxSize = maxSize
	c.memoryLimit = memoryLimit
	c.callbacks = callbacks
	return c
}

// calculateSize estimates the memory size of a value
func (c *Cache) calculateSize(value any) int64 {
	if value == nil {
		return 8 // pointer size
	}

	switch v := value.(type) {
	case string:
		return int64(len(v)) + 16
	case []byte:
		return int64(len(v)) + 24
	case int, int32, int64, float32, float64, bool:
		return 8
	default:
		// For complex types, use reflection to estimate size
		rv := reflect.ValueOf(value)
		return c.reflectSize(rv) + 16 // add overhead
	}
}

func (c *Cache) reflectSize(rv reflect.Value) int64 {
	switch rv.Kind() {
	case reflect.String:
		return int64(rv.Len()) + 16
	case reflect.Slice, reflect.Array:
		size := int64(24) // slice header
		for i := 0; i < rv.Len(); i++ {
			size += c.reflectSize(rv.Index(i))
		}
		return size
	case reflect.Map:
		size := int64(48) // map header
		for _, key := range rv.MapKeys() {
			size += c.reflectSize(key) + c.reflectSize(rv.MapIndex(key))
		}
		return size
	default:
		return int64(unsafe.Sizeof(rv.Interface()))
	}
}

// Get retrieves a value from the cache
func (c *Cache) Get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, found := c.store[key]
	if !found || time.Now().After(item.expiration) {
		if found {
			c.removeItem(key, item, true)
			atomic.AddInt64(&c.stats.Expirations, 1)
		}
		atomic.AddInt64(&c.stats.Misses, 1)
		return "", false
	}

	// Update LRU position
	if item.element != nil {
		c.lruList.MoveToFront(item.element)
	}

	atomic.AddInt64(&c.stats.Hits, 1)
	return item.value, true
}

// Set stores a value in the cache with TTL
func (c *Cache) Set(key string, value any, ttl time.Duration) {
	c.SetWithNamespace(key, value, ttl, "")
}

// SetWithNamespace stores a value in a specific namespace
func (c *Cache) SetWithNamespace(key string, value any, ttl time.Duration, namespace string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	size := c.calculateSize(value)

	// Check memory limit
	if c.memoryLimit > 0 && c.currentSize+size > c.memoryLimit {
		c.evictToFitSize(size)
	}

	// Check size limit and evict LRU if needed
	if c.maxSize > 0 && len(c.store) >= c.maxSize {
		c.evictLRU(1)
	}

	// Remove existing item if present
	if existing, exists := c.store[key]; exists {
		c.removeItem(key, existing, false)
	}

	item := &cacheItem{
		key:         key,
		value:       value,
		expiration:  time.Now().Add(ttl),
		namespace:   namespace,
		originalTTL: ttl,
		size:        size,
	}

	// Add to LRU list
	item.element = c.lruList.PushFront(key)

	// Add to main store
	c.store[key] = item
	c.currentSize += size

	// Add to namespace if specified
	if namespace != "" {
		if c.namespaces[namespace] == nil {
			c.namespaces[namespace] = make(map[string]*cacheItem)
		}
		c.namespaces[namespace][key] = item
	}

	atomic.AddInt64(&c.stats.Sets, 1)

	// Trigger callback
	if c.callbacks != nil && c.callbacks.OnSet != nil {
		go c.callbacks.OnSet(key, value)
	}
}

// Delete removes a key from the cache
func (c *Cache) Delete(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, found := c.store[key]
	if !found {
		return false
	}

	c.removeItem(key, item, false)
	atomic.AddInt64(&c.stats.Deletes, 1)

	if c.callbacks != nil && c.callbacks.OnDelete != nil {
		go c.callbacks.OnDelete(key, item.value)
	}

	return true
}

// removeItem removes an item from all data structures
func (c *Cache) removeItem(key string, item *cacheItem, expired bool) {
	// Remove from main store
	delete(c.store, key)
	c.currentSize -= item.size

	// Remove from LRU list
	if item.element != nil {
		c.lruList.Remove(item.element)
	}

	// Remove from namespace
	if item.namespace != "" {
		if nsMap := c.namespaces[item.namespace]; nsMap != nil {
			delete(nsMap, key)
			if len(nsMap) == 0 {
				delete(c.namespaces, item.namespace)
			}
		}
	}

	// Trigger appropriate callback
	if c.callbacks != nil {
		if expired && c.callbacks.OnExpire != nil {
			go c.callbacks.OnExpire(key, item.value)
		} else if !expired && c.callbacks.OnEvict != nil {
			go c.callbacks.OnEvict(key, item.value)
		}
	}
}

// evictLRU removes the least recently used items
func (c *Cache) evictLRU(count int) {
	for i := 0; i < count && c.lruList.Len() > 0; i++ {
		elem := c.lruList.Back()
		if elem != nil {
			key := elem.Value.(string)
			if item := c.store[key]; item != nil {
				c.removeItem(key, item, false)
				atomic.AddInt64(&c.stats.Evictions, 1)
			}
		}
	}
}

// evictToFitSize evicts items until there's enough space
func (c *Cache) evictToFitSize(neededSize int64) {
	for c.currentSize+neededSize > c.memoryLimit && c.lruList.Len() > 0 {
		c.evictLRU(1)
	}
}

// StartCleanup starts automatic cleanup of expired items
func (c *Cache) StartCleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			c.cleanup()
		}
	}()
}

// cleanup removes expired items
func (c *Cache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	var keysToRemove []string

	for k, v := range c.store {
		if now.After(v.expiration) {
			keysToRemove = append(keysToRemove, k)
		}
	}

	for _, key := range keysToRemove {
		if item := c.store[key]; item != nil {
			c.removeItem(key, item, true)
			atomic.AddInt64(&c.stats.Expirations, 1)
		}
	}
}

// Stats returns current cache statistics
func (c *Cache) Stats() CacheStats {
	return CacheStats{
		Hits:        atomic.LoadInt64(&c.stats.Hits),
		Misses:      atomic.LoadInt64(&c.stats.Misses),
		Sets:        atomic.LoadInt64(&c.stats.Sets),
		Evictions:   atomic.LoadInt64(&c.stats.Evictions),
		Expirations: atomic.LoadInt64(&c.stats.Expirations),
		Deletes:     atomic.LoadInt64(&c.stats.Deletes),
	}
}

// ResetStats resets all statistics counters
func (c *Cache) ResetStats() {
	atomic.StoreInt64(&c.stats.Hits, 0)
	atomic.StoreInt64(&c.stats.Misses, 0)
	atomic.StoreInt64(&c.stats.Sets, 0)
	atomic.StoreInt64(&c.stats.Evictions, 0)
	atomic.StoreInt64(&c.stats.Expirations, 0)
	atomic.StoreInt64(&c.stats.Deletes, 0)
}

// SetCallbacks sets event callback functions
func (c *Cache) SetCallbacks(callbacks *CacheCallbacks) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.callbacks = callbacks
}

// SetMaxSize sets the maximum number of items in the cache
func (c *Cache) SetMaxSize(maxSize int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.maxSize = maxSize

	// Evict excess items if needed
	if maxSize > 0 && len(c.store) > maxSize {
		c.evictLRU(len(c.store) - maxSize)
	}
}

// SetMemoryLimit sets the maximum memory usage in bytes
func (c *Cache) SetMemoryLimit(limit int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.memoryLimit = limit

	// Evict items if over limit
	if limit > 0 && c.currentSize > limit {
		c.evictToFitSize(0)
	}
}

// MemoryUsage returns current memory usage in bytes
func (c *Cache) MemoryUsage() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentSize
}

// ItemCount returns the number of items in the cache
func (c *Cache) ItemCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.store)
}

// SetMultiple sets multiple key-value pairs with the same TTL
func (c *Cache) SetMultiple(items map[string]any, ttl time.Duration) error {
	for key, value := range items {
		c.Set(key, value, ttl)
	}
	return nil
}

// GetMultiple retrieves multiple keys at once
func (c *Cache) GetMultiple(keys []string) map[string]any {
	result := make(map[string]any)
	for _, key := range keys {
		if value, found := c.Get(key); found {
			result[key] = value
		}
	}
	return result
}

// DeleteMultiple removes multiple keys and returns the number deleted
func (c *Cache) DeleteMultiple(keys []string) int {
	count := 0
	for _, key := range keys {
		if c.Delete(key) {
			count++
		}
	}
	return count
}

// GetByPattern returns all key-value pairs matching a pattern
func (c *Cache) GetByPattern(pattern string) map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]any)
	now := time.Now()

	for key, item := range c.store {
		if now.After(item.expiration) {
			continue
		}

		matched, err := filepath.Match(pattern, key)
		if err == nil && matched {
			result[key] = item.value
		}
	}
	return result
}

// GetByRegex returns all key-value pairs where keys match a regex pattern
func (c *Cache) GetByRegex(pattern string) map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()

	regex, err := regexp.Compile(pattern)
	if err != nil {
		return make(map[string]any)
	}

	result := make(map[string]any)
	now := time.Now()

	for key, item := range c.store {
		if now.After(item.expiration) {
			continue
		}

		if regex.MatchString(key) {
			result[key] = item.value
		}
	}
	return result
}

// DeleteByPattern removes all keys matching a pattern
func (c *Cache) DeleteByPattern(pattern string) int {
	keys := c.Keys()
	count := 0

	for _, key := range keys {
		matched, err := filepath.Match(pattern, key)
		if err == nil && matched {
			if c.Delete(key) {
				count++
			}
		}
	}
	return count
}

// Keys returns all cache keys
func (c *Cache) Keys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := make([]string, 0, len(c.store))
	now := time.Now()

	for key, item := range c.store {
		if !now.After(item.expiration) {
			keys = append(keys, key)
		}
	}

	sort.Strings(keys)
	return keys
}

// SaveToFile saves cache contents to a JSON file
func (c *Cache) SaveToFile(filename string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	items := make([]SerializableItem, 0, len(c.store))
	now := time.Now()

	for key, item := range c.store {
		if !now.After(item.expiration) {
			items = append(items, SerializableItem{
				Key:         key,
				Value:       item.value,
				Expiration:  item.expiration,
				Namespace:   item.namespace,
				OriginalTTL: item.originalTTL,
			})
		}
	}

	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0o644)
}

// LoadFromFile loads cache contents from a JSON file
func (c *Cache) LoadFromFile(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	var items []SerializableItem
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}

	now := time.Now()
	for _, item := range items {
		if now.Before(item.Expiration) {
			ttl := time.Until(item.Expiration)
			c.SetWithNamespace(item.Key, item.Value, ttl, item.Namespace)
		}
	}

	return nil
}

// Export returns all cache contents as a map
func (c *Cache) Export() map[string]any {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]any)
	now := time.Now()

	for key, item := range c.store {
		if !now.After(item.expiration) {
			result[key] = item.value
		}
	}

	return result
}

// SetWithAbsoluteExpiry sets a key to expire at a specific time
func (c *Cache) SetWithAbsoluteExpiry(key string, value any, expireAt time.Time) {
	ttl := time.Until(expireAt)
	c.Set(key, value, ttl)
}

// ExtendTTL extends the TTL of an existing key
func (c *Cache) ExtendTTL(key string, additionalTime time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, found := c.store[key]
	if !found || time.Now().After(item.expiration) {
		return false
	}

	item.expiration = item.expiration.Add(additionalTime)
	return true
}

// GetTTL returns the remaining TTL for a key
func (c *Cache) GetTTL(key string) (time.Duration, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	item, found := c.store[key]
	if !found {
		return 0, false
	}

	now := time.Now()
	if now.After(item.expiration) {
		return 0, false
	}

	return time.Until(item.expiration), true
}

// Refresh resets a key's TTL to its original value
func (c *Cache) Refresh(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, found := c.store[key]
	if !found || time.Now().After(item.expiration) {
		return false
	}

	item.expiration = time.Now().Add(item.originalTTL)
	return true
}

// ForEach iterates over all cache items with a callback
func (c *Cache) ForEach(fn func(key string, value any, expiration time.Time) bool) {
	c.mu.RLock()
	snapshot := make(map[string]*cacheItem)
	for k, v := range c.store {
		snapshot[k] = v
	}
	c.mu.RUnlock()

	now := time.Now()
	for key, item := range snapshot {
		if now.After(item.expiration) {
			continue
		}
		if !fn(key, item.value, item.expiration) {
			break
		}
	}
}

// Snapshot returns a copy of all cache contents
func (c *Cache) Snapshot() map[string]any {
	return c.Export()
}

// Namespace returns a namespace-scoped cache interface
func (c *Cache) Namespace(name string) *CacheNamespace {
	return &CacheNamespace{
		cache:     c,
		namespace: name,
	}
}

// ListNamespaces returns all namespace names
func (c *Cache) ListNamespaces() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	namespaces := make([]string, 0, len(c.namespaces))
	for ns := range c.namespaces {
		if ns != "" {
			namespaces = append(namespaces, ns)
		}
	}

	sort.Strings(namespaces)
	return namespaces
}

// ClearNamespace removes all items from a namespace
func (c *Cache) ClearNamespace(namespace string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	nsMap := c.namespaces[namespace]
	if nsMap == nil {
		return 0
	}

	count := 0
	for key := range nsMap {
		if item := c.store[key]; item != nil {
			c.removeItem(key, item, false)
			count++
		}
	}

	return count
}

// Clear removes all items from the cache
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.store = make(map[string]*cacheItem)
	c.lruList = list.New()
	c.namespaces = make(map[string]map[string]*cacheItem)
	c.currentSize = 0
}

// CacheNamespace methods

// Get retrieves a value from the namespace
func (cn *CacheNamespace) Get(key string) (any, bool) {
	return cn.cache.Get(cn.namespacedKey(key))
}

// Set stores a value in the namespace
func (cn *CacheNamespace) Set(key string, value any, ttl time.Duration) {
	cn.cache.SetWithNamespace(cn.namespacedKey(key), value, ttl, cn.namespace)
}

// Delete removes a key from the namespace
func (cn *CacheNamespace) Delete(key string) bool {
	return cn.cache.Delete(cn.namespacedKey(key))
}

// Keys returns all keys in the namespace
func (cn *CacheNamespace) Keys() []string {
	cn.cache.mu.RLock()
	defer cn.cache.mu.RUnlock()

	nsMap := cn.cache.namespaces[cn.namespace]
	if nsMap == nil {
		return []string{}
	}

	keys := make([]string, 0, len(nsMap))
	now := time.Now()

	for key, item := range nsMap {
		if !now.After(item.expiration) {
			// Remove namespace prefix
			if strings.HasPrefix(key, cn.namespace+":") {
				keys = append(keys, key[len(cn.namespace)+1:])
			}
		}
	}

	sort.Strings(keys)
	return keys
}

// Clear removes all items from the namespace
func (cn *CacheNamespace) Clear() int {
	return cn.cache.ClearNamespace(cn.namespace)
}

// ItemCount returns the number of items in the namespace
func (cn *CacheNamespace) ItemCount() int {
	cn.cache.mu.RLock()
	defer cn.cache.mu.RUnlock()

	nsMap := cn.cache.namespaces[cn.namespace]
	if nsMap == nil {
		return 0
	}

	count := 0
	now := time.Now()
	for _, item := range nsMap {
		if !now.After(item.expiration) {
			count++
		}
	}

	return count
}

func (cn *CacheNamespace) namespacedKey(key string) string {
	return cn.namespace + ":" + key
}

// Utility functions

// GCStats triggers garbage collection and returns memory stats
func (c *Cache) GCStats() (before, after runtime.MemStats) {
	runtime.ReadMemStats(&before)
	runtime.GC()
	runtime.ReadMemStats(&after)
	return
}

// HealthCheck returns cache health information
func (c *Cache) HealthCheck() map[string]any {
	stats := c.Stats()

	hitRatio := float64(0)
	total := stats.Hits + stats.Misses
	if total > 0 {
		hitRatio = float64(stats.Hits) / float64(total)
	}

	return map[string]any{
		"item_count":   c.ItemCount(),
		"memory_usage": c.MemoryUsage(),
		"memory_limit": c.memoryLimit,
		"max_size":     c.maxSize,
		"hit_ratio":    hitRatio,
		"stats":        stats,
		"namespaces":   len(c.namespaces),
	}
}
