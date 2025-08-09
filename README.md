# Enhanced Cache Features Documentation

This document describes the comprehensive cache implementation with 10 advanced features that make it production-ready for various use cases.

## Table of Contents

1. [Core Features](#core-features)
2. [LRU Eviction & Size Limiting](#1-lru-eviction--size-limiting)
3. [Statistics & Metrics](#2-statistics--metrics)
4. [Event Callbacks](#3-event-callbacks)
5. [Batch Operations](#4-batch-operations)
6. [Pattern-Based Operations](#5-pattern-based-operations)
7. [Persistence](#6-persistence)
8. [Advanced TTL Management](#7-advanced-ttl-management)
9. [Memory Usage Monitoring](#8-memory-usage-monitoring)
10. [Thread-Safe Iteration](#9-thread-safe-iteration)
11. [Namespace Support](#10-namespace-support)
12. [Performance Benchmarks](#performance-benchmarks)
13. [Usage Examples](#usage-examples)

## Core Features

The enhanced cache maintains all original functionality while adding advanced features:

- **Thread-safe**: All operations are protected by RWMutex
- **TTL support**: Automatic expiration of items
- **Automatic cleanup**: Background goroutine removes expired items
- **Type-agnostic**: Stores any Go type using `any` interface

## 1. LRU Eviction & Size Limiting

### Features
- **Maximum item limit**: Set a maximum number of cached items
- **LRU eviction**: Automatically removes least recently used items when limit is exceeded
- **Memory limit**: Set maximum memory usage with automatic eviction
- **Dynamic sizing**: Adjust limits at runtime

### API Methods
```go
// Create cache with limits
cache := NewCacheWithOptions(maxSize, memoryLimit, callbacks)

// Set limits dynamically
cache.SetMaxSize(1000)
cache.SetMemoryLimit(1024 * 1024) // 1MB

// Monitor usage
itemCount := cache.ItemCount()
memoryUsage := cache.MemoryUsage()
```

### Example
```go
// Cache with max 1000 items and 10MB memory limit
cache := NewCacheWithOptions(1000, 10*1024*1024, nil)

// Items will be automatically evicted when limits are exceeded
for i := 0; i < 1500; i++ {
    cache.Set(fmt.Sprintf("key_%d", i), "value", time.Minute)
}
// Only 1000 most recent items will remain
```

## 2. Statistics & Metrics

### Features
- **Performance tracking**: Hits, misses, sets, evictions, expirations, deletes
- **Hit ratio calculation**: Automatic calculation of cache efficiency
- **Thread-safe counters**: Uses atomic operations for accuracy
- **Reset capability**: Clear statistics without affecting cache data

### API Methods
```go
type CacheStats struct {
    Hits        int64
    Misses      int64
    Sets        int64
    Evictions   int64
    Expirations int64
    Deletes     int64
}

stats := cache.Stats()
cache.ResetStats()
```

### Example
```go
cache := NewCache()
cache.Set("key", "value", time.Minute)
cache.Get("key")        // Hit
cache.Get("missing")    // Miss

stats := cache.Stats()
fmt.Printf("Hit ratio: %.2f%%", float64(stats.Hits)/float64(stats.Hits+stats.Misses)*100)
```

## 3. Event Callbacks

### Features
- **Event hooks**: React to cache operations (set, expire, evict, delete)
- **Asynchronous execution**: Callbacks run in separate goroutines
- **Flexible handlers**: Customize behavior for each event type
- **Runtime configuration**: Set or change callbacks at runtime

### API Methods
```go
type CacheCallbacks struct {
    OnExpire func(key string, value any)
    OnEvict  func(key string, value any)
    OnSet    func(key string, value any)
    OnDelete func(key string, value any)
}

callbacks := &CacheCallbacks{
    OnExpire: func(key string, value any) {
        log.Printf("Key expired: %s", key)
    },
}

cache.SetCallbacks(callbacks)
```

### Example
```go
callbacks := &CacheCallbacks{
    OnSet: func(key string, value any) {
        log.Printf("Item cached: %s", key)
    },
    OnExpire: func(key string, value any) {
        log.Printf("Item expired: %s", key)
    },
    OnEvict: func(key string, value any) {
        metrics.IncrementEvictions()
    },
}

cache := NewCacheWithOptions(100, 0, callbacks)
```

## 4. Batch Operations

### Features
- **Bulk set**: Set multiple key-value pairs efficiently
- **Bulk get**: Retrieve multiple keys in one operation
- **Bulk delete**: Remove multiple keys and get count of deletions
- **Error handling**: Graceful handling of partial failures

### API Methods
```go
// Batch set
err := cache.SetMultiple(map[string]any{
    "key1": "value1",
    "key2": "value2",
}, time.Minute)

// Batch get
results := cache.GetMultiple([]string{"key1", "key2", "key3"})

// Batch delete
deleted := cache.DeleteMultiple([]string{"key1", "key2"})
```

### Example
```go
// Batch operations for better performance
users := map[string]any{
    "user:1": User{Name: "John", Email: "john@example.com"},
    "user:2": User{Name: "Jane", Email: "jane@example.com"},
    "user:3": User{Name: "Bob",  Email: "bob@example.com"},
}

cache.SetMultiple(users, time.Hour)

// Later, get multiple users
userKeys := []string{"user:1", "user:2", "user:3"}
foundUsers := cache.GetMultiple(userKeys)
```

## 5. Pattern-Based Operations

### Features
- **Wildcard matching**: Use filesystem-style patterns (`*`, `?`, `[abc]`)
- **Regex support**: Full regular expression pattern matching
- **Bulk operations**: Get or delete all keys matching a pattern
- **Sorted results**: Keys returned in sorted order

### API Methods
```go
// Pattern matching (glob style)
matches := cache.GetByPattern("user:*")
deleted := cache.DeleteByPattern("temp:*")

// Regex matching
matches := cache.GetByRegex("^session_[0-9]+$")

// Get all keys
allKeys := cache.Keys()
```

### Example
```go
// Cache some user and session data
cache.Set("user:1", "John", time.Hour)
cache.Set("user:2", "Jane", time.Hour)
cache.Set("session:abc123", "session_data", time.Minute)
cache.Set("session:def456", "session_data", time.Minute)

// Get all users
users := cache.GetByPattern("user:*")

// Clean up expired sessions
expiredSessions := cache.DeleteByPattern("session:*")

// Use regex for more complex patterns
adminUsers := cache.GetByRegex("^admin_user_[0-9]+$")
```

## 6. Persistence

### Features
- **JSON serialization**: Save/load cache contents to/from JSON files
- **Selective loading**: Only loads non-expired items
- **Export capability**: Export current cache state as map
- **Error handling**: Graceful handling of file I/O errors

### API Methods
```go
// Save cache to file
err := cache.SaveToFile("cache_backup.json")

// Load cache from file
err := cache.LoadFromFile("cache_backup.json")

// Export as map
data := cache.Export()
```

### Example
```go
// Save cache state before shutdown
cache.Set("config:timeout", "30s", time.Hour)
cache.Set("config:retries", 3, time.Hour)

err := cache.SaveToFile("app_cache.json")
if err != nil {
    log.Printf("Failed to save cache: %v", err)
}

// Load cache state on startup
newCache := NewCache()
err = newCache.LoadFromFile("app_cache.json")
if err != nil {
    log.Printf("Failed to load cache: %v", err)
}
```

## 7. Advanced TTL Management

### Features
- **Absolute expiration**: Set items to expire at specific times
- **TTL extension**: Extend the lifetime of existing items
- **TTL querying**: Check remaining time for any key
- **TTL refresh**: Reset item's TTL to its original value

### API Methods
```go
// Set with absolute expiration time
cache.SetWithAbsoluteExpiry("key", "value", time.Now().Add(time.Hour))

// Extend TTL
extended := cache.ExtendTTL("key", 30*time.Minute)

// Check remaining TTL
ttl, exists := cache.GetTTL("key")

// Refresh to original TTL
refreshed := cache.Refresh("key")
```

### Example
```go
// Cache data with specific expiration
endOfDay := time.Date(2024, time.January, 1, 23, 59, 59, 0, time.UTC)
cache.SetWithAbsoluteExpiry("daily_stats", stats, endOfDay)

// Extend session if user is active
if userIsActive("session_123") {
    cache.ExtendTTL("session_123", 30*time.Minute)
}

// Check if item is about to expire
if ttl, exists := cache.GetTTL("important_data"); exists && ttl < 5*time.Minute {
    // Refresh the data
    cache.Refresh("important_data")
}
```

## 8. Memory Usage Monitoring

### Features
- **Memory tracking**: Approximate memory usage calculation
- **Memory limits**: Automatic eviction when limits exceeded
- **Size estimation**: Smart size calculation for different data types
- **Runtime monitoring**: Check current memory usage

### API Methods
```go
// Set memory limit (in bytes)
cache.SetMemoryLimit(10 * 1024 * 1024) // 10MB

// Check current usage
usage := cache.MemoryUsage()

// Get item count
count := cache.ItemCount()
```

### Example
```go
// Create cache with 5MB limit
cache := NewCacheWithOptions(0, 5*1024*1024, nil)

// Monitor memory usage
for i := 0; i < 1000; i++ {
    largeData := make([]byte, 1024) // 1KB each
    cache.Set(fmt.Sprintf("data_%d", i), largeData, time.Hour)
    
    if i%100 == 0 {
        usage := cache.MemoryUsage()
        fmt.Printf("Memory usage: %d bytes\n", usage)
    }
}
```

## 9. Thread-Safe Iteration

### Features
- **Safe iteration**: Iterate over cache without blocking other operations
- **Snapshot capability**: Get consistent view of cache at point in time
- **Early termination**: Break out of iteration based on conditions
- **Non-blocking**: Uses snapshots to avoid holding locks

### API Methods
```go
// Iterate with callback
cache.ForEach(func(key string, value any, expiration time.Time) bool {
    fmt.Printf("Key: %s, Value: %v\n", key, value)
    return true // continue iteration
})

// Get snapshot
snapshot := cache.Snapshot()
```

### Example
```go
// Process all cached users
cache.ForEach(func(key string, value any, expiration time.Time) bool {
    if strings.HasPrefix(key, "user:") {
        user := value.(User)
        processUser(user)
    }
    return true // continue
})

// Get snapshot for reporting
snapshot := cache.Snapshot()
report := generateReport(snapshot)
```

## 10. Namespace Support

### Features
- **Logical separation**: Separate cache spaces for different data types
- **Isolated operations**: Operations within namespace don't affect others
- **Namespace management**: List, clear, and manage namespaces
- **Scoped interface**: Namespace-specific cache interface

### API Methods
```go
// Get namespace interface
userCache := cache.Namespace("users")
sessionCache := cache.Namespace("sessions")

// Namespace-scoped operations
userCache.Set("123", userData, time.Hour)
sessionCache.Set("abc", sessionData, time.Minute)

// Namespace management
namespaces := cache.ListNamespaces()
cleared := cache.ClearNamespace("users")
```

### Example
```go
cache := NewCache()

// Create separate namespaces
userCache := cache.Namespace("users")
sessionCache := cache.Namespace("sessions")
configCache := cache.Namespace("config")

// Each namespace operates independently
userCache.Set("1", User{Name: "John"}, time.Hour)
sessionCache.Set("1", Session{Token: "abc123"}, time.Minute)
configCache.Set("1", Config{Timeout: 30}, time.Hour*24)

// Keys don't conflict between namespaces
user, _ := userCache.Get("1")     // Gets User
session, _ := sessionCache.Get("1") // Gets Session
config, _ := configCache.Get("1")   // Gets Config

// Clean up specific namespace
sessionCache.Clear()
```

## Performance Benchmarks

Based on the benchmark results on Apple M4 Max:

- **Set operations**: ~347 ns/op (294 B/op, 5 allocs/op)
- **Get operations**: ~86 ns/op (21 B/op, 1 alloc/op)
- **Concurrent operations**: ~389 ns/op (114 B/op, 3 allocs/op)
- **Namespace operations**: ~554 ns/op (395 B/op, 6 allocs/op)
- **Pattern matching**: ~70.8 μs/op for 1000 items
- **Batch operations**: ~13.6 μs/op for 100 items
- **Memory tracking**: ~408 ns/op (388 B/op, 7 allocs/op)

## Usage Examples

### Web Application Cache

```go
// Configure cache for web application
callbacks := &CacheCallbacks{
    OnEvict: func(key string, value any) {
        metrics.IncrementCacheEvictions()
    },
    OnExpire: func(key string, value any) {
        log.Printf("Cache item expired: %s", key)
    },
}

appCache := NewCacheWithOptions(10000, 50*1024*1024, callbacks)

// Cache user sessions
sessionCache := appCache.Namespace("sessions")
sessionCache.Set(sessionID, sessionData, 30*time.Minute)

// Cache database query results
queryCache := appCache.Namespace("queries")
queryCache.Set(queryHash, results, 5*time.Minute)

// Cache static content
staticCache := appCache.Namespace("static")
staticCache.Set(fileName, fileContent, time.Hour)
```

### Microservice Cache

```go
// Service-specific cache with persistence
cache := NewCacheWithOptions(5000, 20*1024*1024, nil)

// Load previous cache state
if err := cache.LoadFromFile("service_cache.json"); err != nil {
    log.Printf("Could not load cache: %v", err)
}

// Cache API responses
cache.Set("api:users:list", usersList, 10*time.Minute)

// Cache with pattern-based cleanup
cache.Set("temp:process:123", processData, 5*time.Minute)

// Clean up temporary data periodically
go func() {
    ticker := time.NewTicker(time.Minute)
    for range ticker.C {
        deleted := cache.DeleteByPattern("temp:*")
        if deleted > 0 {
            log.Printf("Cleaned up %d temporary items", deleted)
        }
    }
}()

// Save state before shutdown
defer cache.SaveToFile("service_cache.json")
```

### High-Performance Cache

```go
// Optimized for high throughput
cache := NewCacheWithOptions(100000, 100*1024*1024, nil)

// Batch operations for efficiency
items := make(map[string]any)
for i := 0; i < 1000; i++ {
    items[fmt.Sprintf("batch_key_%d", i)] = generateData(i)
}
cache.SetMultiple(items, time.Hour)

// Concurrent access patterns
var wg sync.WaitGroup
for i := 0; i < 10; i++ {
    wg.Add(1)
    go func(worker int) {
        defer wg.Done()
        for j := 0; j < 10000; j++ {
            key := fmt.Sprintf("worker_%d_item_%d", worker, j)
            cache.Set(key, generateData(j), time.Minute)
        }
    }(i)
}
wg.Wait()

// Monitor performance
stats := cache.Stats()
hitRatio := float64(stats.Hits) / float64(stats.Hits + stats.Misses)
log.Printf("Cache hit ratio: %.2f%%", hitRatio*100)
```

## Health Monitoring

```go
// Health check endpoint
func cacheHealthHandler(w http.ResponseWriter, r *http.Request) {
    health := cache.HealthCheck()
    
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(health)
}

// Example health response:
// {
//   "item_count": 1500,
//   "memory_usage": 1024000,
//   "memory_limit": 10485760,
//   "max_size": 10000,
//   "hit_ratio": 0.85,
//   "stats": {
//     "hits": 8500,
//     "misses": 1500,
//     "sets": 2000,
//     "evictions": 50,
//     "expirations": 200,
//     "deletes": 100
//   },
//   "namespaces": 3
// }
```

## Best Practices

1. **Choose appropriate limits**: Set `maxSize` and `memoryLimit` based on your application's needs
2. **Use namespaces**: Organize related data in separate namespaces for better management
3. **Monitor statistics**: Regularly check hit ratios and adjust cache settings
4. **Handle callbacks efficiently**: Keep callback functions lightweight to avoid blocking
5. **Use batch operations**: For bulk operations, use `SetMultiple` and `GetMultiple`
6. **Implement persistence**: Save important cache data across application restarts
7. **Clean up patterns**: Use pattern-based deletion for temporary data cleanup
8. **Monitor memory usage**: Set appropriate memory limits and monitor usage patterns
9. **Use appropriate TTLs**: Balance between data freshness and cache efficiency
10. **Test thoroughly**: Use the comprehensive test suite as a reference for proper usage

This enhanced cache implementation provides enterprise-grade features while maintaining simplicity and high performance, making it suitable for a wide range of applications from simple web services to complex distributed systems.