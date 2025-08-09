package cache

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewCache(t *testing.T) {
	cache := NewCache()
	if cache == nil {
		t.Fatal("NewCache() returned nil")
	}
	if cache.store == nil {
		t.Fatal("cache.store is nil")
	}
}

func TestCacheSetAndGet(t *testing.T) {
	cache := NewCache()

	// Test setting and getting a value
	key := "test_key"
	value := "test_value"
	ttl := 5 * time.Second

	cache.Set(key, value, ttl)

	retrieved, found := cache.Get(key)
	if !found {
		t.Fatalf("Expected to find key %s", key)
	}
	if retrieved != value {
		t.Fatalf("Expected %v, got %v", value, retrieved)
	}
}

func TestCacheGetNonExistentKey(t *testing.T) {
	cache := NewCache()

	value, found := cache.Get("non_existent_key")
	if found {
		t.Fatal("Expected not to find non-existent key")
	}
	if value != "" {
		t.Fatalf("Expected empty string for non-existent key, got %v", value)
	}
}

func TestCacheExpiration(t *testing.T) {
	cache := NewCache()

	key := "expiring_key"
	value := "expiring_value"
	ttl := 100 * time.Millisecond

	cache.Set(key, value, ttl)

	// Value should be available immediately
	retrieved, found := cache.Get(key)
	if !found {
		t.Fatal("Expected to find key immediately after setting")
	}
	if retrieved != value {
		t.Fatalf("Expected %v, got %v", value, retrieved)
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Value should be expired
	_, found = cache.Get(key)
	if found {
		t.Fatal("Expected key to be expired")
	}
}

func TestCacheOverwrite(t *testing.T) {
	cache := NewCache()

	key := "overwrite_key"
	value1 := "first_value"
	value2 := "second_value"
	ttl := 5 * time.Second

	// Set first value
	cache.Set(key, value1, ttl)
	retrieved, found := cache.Get(key)
	if !found || retrieved != value1 {
		t.Fatalf("Expected %v, got %v", value1, retrieved)
	}

	// Overwrite with second value
	cache.Set(key, value2, ttl)
	retrieved, found = cache.Get(key)
	if !found || retrieved != value2 {
		t.Fatalf("Expected %v, got %v", value2, retrieved)
	}
}

func TestCacheWithDifferentTypes(t *testing.T) {
	cache := NewCache()
	ttl := 5 * time.Second

	// Test with string
	cache.Set("string_key", "string_value", ttl)
	val, found := cache.Get("string_key")
	if !found || val != "string_value" {
		t.Fatalf("String test failed: expected 'string_value', got %v", val)
	}

	// Test with int
	cache.Set("int_key", 42, ttl)
	val, found = cache.Get("int_key")
	if !found || val != 42 {
		t.Fatalf("Int test failed: expected 42, got %v", val)
	}

	// Test with slice
	slice := []string{"a", "b", "c"}
	cache.Set("slice_key", slice, ttl)
	val, found = cache.Get("slice_key")
	if !found {
		t.Fatal("Slice test failed: key not found")
	}
	retrievedSlice, ok := val.([]string)
	if !ok {
		t.Fatal("Slice test failed: type assertion failed")
	}
	if len(retrievedSlice) != 3 || retrievedSlice[0] != "a" {
		t.Fatalf("Slice test failed: expected %v, got %v", slice, retrievedSlice)
	}
}

func TestCacheCleanup(t *testing.T) {
	cache := NewCache()

	// Add some items with short TTL
	cache.Set("key1", "value1", 50*time.Millisecond)
	cache.Set("key2", "value2", 50*time.Millisecond)
	cache.Set("key3", "value3", 5*time.Second) // This one should survive

	// Wait for first two to expire
	time.Sleep(100 * time.Millisecond)

	// Manually trigger cleanup
	cache.cleanup()

	// Check that expired items are removed
	_, found1 := cache.Get("key1")
	_, found2 := cache.Get("key2")
	_, found3 := cache.Get("key3")

	if found1 {
		t.Fatal("Expected key1 to be cleaned up")
	}
	if found2 {
		t.Fatal("Expected key2 to be cleaned up")
	}
	if !found3 {
		t.Fatal("Expected key3 to still exist")
	}

	// Verify internal store size
	cache.mu.Lock()
	storeSize := len(cache.store)
	cache.mu.Unlock()

	if storeSize != 1 {
		t.Fatalf("Expected store size to be 1 after cleanup, got %d", storeSize)
	}
}

func TestCacheStartCleanup(t *testing.T) {
	cache := NewCache()

	// Add items with short TTL
	cache.Set("cleanup_key1", "value1", 50*time.Millisecond)
	cache.Set("cleanup_key2", "value2", 50*time.Millisecond)

	// Start automatic cleanup with short interval
	cache.StartCleanup(75 * time.Millisecond)

	// Wait for cleanup to run
	time.Sleep(200 * time.Millisecond)

	// Check that items are cleaned up
	_, found1 := cache.Get("cleanup_key1")
	_, found2 := cache.Get("cleanup_key2")

	if found1 || found2 {
		t.Fatal("Expected automatic cleanup to remove expired items")
	}
}

func TestCacheConcurrency(t *testing.T) {
	cache := NewCache()

	const numGoroutines = 10
	const numOperations = 100

	var wg sync.WaitGroup

	// Start multiple goroutines that set values
	for i := range numGoroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range numOperations {
				key := fmt.Sprintf("key_%d_%d", id, j)
				value := fmt.Sprintf("value_%d_%d", id, j)
				cache.Set(key, value, 5*time.Second)
			}
		}(i)
	}

	// Start multiple goroutines that read values
	for i := range numGoroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range numOperations {
				key := fmt.Sprintf("key_%d_%d", id, j)
				cache.Get(key)
			}
		}(i)
	}

	wg.Wait()

	// Test should complete without race conditions or panics
}

func TestCacheZeroTTL(t *testing.T) {
	cache := NewCache()

	// Set with zero TTL (should expire immediately)
	cache.Set("zero_ttl_key", "value", 0)

	// Should be expired immediately
	_, found := cache.Get("zero_ttl_key")
	if found {
		t.Fatal("Expected key with zero TTL to be expired immediately")
	}
}

func TestCacheNegativeTTL(t *testing.T) {
	cache := NewCache()

	// Set with negative TTL (should be expired)
	cache.Set("negative_ttl_key", "value", -time.Second)

	// Should be expired immediately
	_, found := cache.Get("negative_ttl_key")
	if found {
		t.Fatal("Expected key with negative TTL to be expired immediately")
	}
}

func BenchmarkCacheSet(b *testing.B) {
	cache := NewCache()
	ttl := 5 * time.Second

	for i := 0; b.Loop(); i++ {
		key := fmt.Sprintf("bench_key_%d", i)
		cache.Set(key, "benchmark_value", ttl)
	}
}

func BenchmarkCacheGet(b *testing.B) {
	cache := NewCache()
	ttl := 5 * time.Second

	// Pre-populate cache
	for i := range 1000 {
		key := fmt.Sprintf("bench_key_%d", i)
		cache.Set(key, "benchmark_value", ttl)
	}

	for i := 0; b.Loop(); i++ {
		key := fmt.Sprintf("bench_key_%d", i%1000)
		cache.Get(key)
	}
}

func BenchmarkCacheConcurrent(b *testing.B) {
	cache := NewCache()
	ttl := 5 * time.Second

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("concurrent_key_%d", i)
			if i%2 == 0 {
				cache.Set(key, "concurrent_value", ttl)
			} else {
				cache.Get(key)
			}
			i++
		}
	})
}

// Test LRU Eviction
func TestCacheLRUEviction(t *testing.T) {
	cache := NewCacheWithOptions(3, 0, nil)

	// Fill cache to capacity
	cache.Set("key1", "value1", 5*time.Second)
	cache.Set("key2", "value2", 5*time.Second)
	cache.Set("key3", "value3", 5*time.Second)

	// Access key1 to make it most recently used
	cache.Get("key1")

	// Add a new item, should evict key2 (least recently used)
	cache.Set("key4", "value4", 5*time.Second)

	// key2 should be evicted
	_, found2 := cache.Get("key2")
	if found2 {
		t.Error("Expected key2 to be evicted")
	}

	// Other keys should still exist
	_, found1 := cache.Get("key1")
	_, found3 := cache.Get("key3")
	_, found4 := cache.Get("key4")

	if !found1 || !found3 || !found4 {
		t.Error("Expected key1, key3, and key4 to still exist")
	}
}

func TestCacheSetMaxSize(t *testing.T) {
	cache := NewCache()

	// Add items
	cache.Set("key1", "value1", 5*time.Second)
	cache.Set("key2", "value2", 5*time.Second)
	cache.Set("key3", "value3", 5*time.Second)

	// Set max size to 2, should evict 1 item
	cache.SetMaxSize(2)

	if cache.ItemCount() != 2 {
		t.Errorf("Expected 2 items, got %d", cache.ItemCount())
	}
}

// Test Statistics
func TestCacheStats(t *testing.T) {
	cache := NewCache()

	// Perform various operations
	cache.Set("key1", "value1", 5*time.Second)
	cache.Set("key2", "value2", 5*time.Second)

	cache.Get("key1")        // Hit
	cache.Get("nonexistent") // Miss

	cache.Delete("key2")

	stats := cache.Stats()

	if stats.Sets != 2 {
		t.Errorf("Expected 2 sets, got %d", stats.Sets)
	}
	if stats.Hits != 1 {
		t.Errorf("Expected 1 hit, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.Misses)
	}
	if stats.Deletes != 1 {
		t.Errorf("Expected 1 delete, got %d", stats.Deletes)
	}
}

func TestCacheResetStats(t *testing.T) {
	cache := NewCache()

	cache.Set("key1", "value1", 5*time.Second)
	cache.Get("key1")

	cache.ResetStats()
	stats := cache.Stats()

	if stats.Sets != 0 || stats.Hits != 0 {
		t.Error("Expected all stats to be reset to 0")
	}
}

// Test Callbacks
func TestCacheCallbacks(t *testing.T) {
	var setKey, expireKey, evictKey, deleteKey string
	var setValue, expireValue, deleteValue any

	callbacks := &CacheCallbacks{
		OnSet: func(key string, value any) {
			setKey = key
			setValue = value
		},
		OnExpire: func(key string, value any) {
			expireKey = key
			expireValue = value
		},
		OnEvict: func(key string, value any) {
			evictKey = key
		},
		OnDelete: func(key string, value any) {
			deleteKey = key
			deleteValue = value
		},
	}

	cache := NewCacheWithOptions(2, 0, callbacks)

	// Test OnSet callback
	cache.Set("testkey", "testvalue", 5*time.Second)
	time.Sleep(10 * time.Millisecond) // Wait for goroutine

	if setKey != "testkey" || setValue != "testvalue" {
		t.Errorf("OnSet callback not triggered correctly: key=%s, value=%v", setKey, setValue)
	}

	// Test OnDelete callback
	cache.Delete("testkey")
	time.Sleep(10 * time.Millisecond)

	if deleteKey != "testkey" || deleteValue != "testvalue" {
		t.Errorf("OnDelete callback not triggered correctly: key=%s, value=%v", deleteKey, deleteValue)
	}

	// Test OnEvict callback
	cache.Set("key1", "value1", 5*time.Second)
	cache.Set("key2", "value2", 5*time.Second)
	cache.Set("key3", "value3", 5*time.Second) // This should trigger eviction
	time.Sleep(10 * time.Millisecond)

	if evictKey == "" {
		t.Error("OnEvict callback not triggered")
	}

	// Test OnExpire callback
	cache.Set("expiring", "expvalue", 50*time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	cache.Get("expiring") // This should trigger expiration detection
	time.Sleep(10 * time.Millisecond)

	if expireKey != "expiring" || expireValue != "expvalue" {
		t.Errorf("OnExpire callback not triggered correctly: key=%s, value=%v", expireKey, expireValue)
	}
}

// Test Batch Operations
func TestCacheBatchOperations(t *testing.T) {
	cache := NewCache()

	// Test SetMultiple
	items := map[string]any{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}

	err := cache.SetMultiple(items, 5*time.Second)
	if err != nil {
		t.Fatalf("SetMultiple failed: %v", err)
	}

	// Test GetMultiple
	keys := []string{"key1", "key2", "key3", "nonexistent"}
	results := cache.GetMultiple(keys)

	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}

	for key, expectedValue := range items {
		if results[key] != expectedValue {
			t.Errorf("Expected %v for key %s, got %v", expectedValue, key, results[key])
		}
	}

	// Test DeleteMultiple
	keysToDelete := []string{"key1", "key2", "nonexistent"}
	deleted := cache.DeleteMultiple(keysToDelete)

	if deleted != 2 {
		t.Errorf("Expected 2 deletions, got %d", deleted)
	}

	// Verify deletions
	_, found1 := cache.Get("key1")
	_, found2 := cache.Get("key2")
	_, found3 := cache.Get("key3")

	if found1 || found2 {
		t.Error("Expected key1 and key2 to be deleted")
	}
	if !found3 {
		t.Error("Expected key3 to still exist")
	}
}

// Test Pattern Operations
func TestCachePatternOperations(t *testing.T) {
	cache := NewCache()

	// Set up test data
	cache.Set("user:1", "John", 5*time.Second)
	cache.Set("user:2", "Jane", 5*time.Second)
	cache.Set("product:1", "Widget", 5*time.Second)
	cache.Set("product:2", "Gadget", 5*time.Second)
	cache.Set("config:timeout", "30s", 5*time.Second)

	// Test GetByPattern
	userResults := cache.GetByPattern("user:*")
	if len(userResults) != 2 {
		t.Errorf("Expected 2 user results, got %d", len(userResults))
	}

	productResults := cache.GetByPattern("product:*")
	if len(productResults) != 2 {
		t.Errorf("Expected 2 product results, got %d", len(productResults))
	}

	// Test GetByRegex
	regexResults := cache.GetByRegex("^user:")
	if len(regexResults) != 2 {
		t.Errorf("Expected 2 regex results, got %d", len(regexResults))
	}

	// Test DeleteByPattern
	deleted := cache.DeleteByPattern("user:*")
	if deleted != 2 {
		t.Errorf("Expected 2 deletions, got %d", deleted)
	}

	// Verify deletions
	remainingUsers := cache.GetByPattern("user:*")
	if len(remainingUsers) != 0 {
		t.Errorf("Expected 0 remaining users, got %d", len(remainingUsers))
	}
}

func TestCacheKeys(t *testing.T) {
	cache := NewCache()

	cache.Set("key1", "value1", 5*time.Second)
	cache.Set("key2", "value2", 5*time.Second)
	cache.Set("key3", "value3", 5*time.Second)

	keys := cache.Keys()
	if len(keys) != 3 {
		t.Errorf("Expected 3 keys, got %d", len(keys))
	}

	// Keys should be sorted
	expected := []string{"key1", "key2", "key3"}
	for i, key := range keys {
		if key != expected[i] {
			t.Errorf("Expected key %s at position %d, got %s", expected[i], i, key)
		}
	}
}

// Test SaveTo/LoadFrom with io.Writer/Reader
func TestCacheSaveLoadIO(t *testing.T) {
	cache := NewCache()

	// Set up test data
	cache.Set("key1", "value1", 5*time.Second)
	cache.Set("key2", 42, 5*time.Second)
	cache.Set("key3", []string{"a", "b", "c"}, 5*time.Second)

	// Set namespaced data using proper namespace interface
	userNS := cache.Namespace("namespace1")
	userNS.Set("nskey1", "nsvalue1", 10*time.Second)

	// Save to buffer
	var buf bytes.Buffer
	err := cache.SaveTo(&buf)
	if err != nil {
		t.Fatalf("SaveTo failed: %v", err)
	}

	// Verify JSON content is not empty
	if buf.Len() == 0 {
		t.Fatal("SaveTo produced empty output")
	}

	// Create new cache and load from buffer
	newCache := NewCache()
	err = newCache.LoadFrom(&buf)
	if err != nil {
		t.Fatalf("LoadFrom failed: %v", err)
	}

	// Verify loaded data
	value1, found1 := newCache.Get("key1")
	value2, found2 := newCache.Get("key2")
	_, found3 := newCache.Get("key3")

	// Get namespaced data using proper namespace interface
	newUserNS := newCache.Namespace("namespace1")
	nsValue1, foundNs1 := newUserNS.Get("nskey1")

	if !found1 || value1 != "value1" {
		t.Error("key1 not loaded correctly")
	}
	if !found2 || value2 != float64(42) { // JSON unmarshaling converts numbers to float64
		t.Errorf("key2 not loaded correctly, got %v (%T)", value2, value2)
	}
	if !found3 {
		t.Error("key3 not found after loading")
	}
	if !foundNs1 || nsValue1 != "nsvalue1" {
		t.Errorf("namespaced key not loaded correctly, got %v, found: %v", nsValue1, foundNs1)
	}
}

// Test SaveTo/LoadFrom with expired items
func TestCacheSaveLoadIOExpiredItems(t *testing.T) {
	cache := NewCache()

	// Set up test data with very short TTL
	cache.Set("expired", "value", 1*time.Millisecond)
	cache.Set("valid", "value", 5*time.Second)

	// Wait for expiration
	time.Sleep(5 * time.Millisecond)

	// Save to buffer
	var buf bytes.Buffer
	err := cache.SaveTo(&buf)
	if err != nil {
		t.Fatalf("SaveTo failed: %v", err)
	}

	// Create new cache and load from buffer
	newCache := NewCache()
	err = newCache.LoadFrom(&buf)
	if err != nil {
		t.Fatalf("LoadFrom failed: %v", err)
	}

	// Verify expired item was not saved/loaded
	_, foundExpired := newCache.Get("expired")
	_, foundValid := newCache.Get("valid")

	if foundExpired {
		t.Error("expired item should not have been saved/loaded")
	}
	if !foundValid {
		t.Error("valid item should have been saved/loaded")
	}
}

// Test LoadFrom with invalid JSON
func TestCacheLoadFromInvalidJSON(t *testing.T) {
	cache := NewCache()

	invalidJSON := strings.NewReader("invalid json content")
	err := cache.LoadFrom(invalidJSON)

	if err == nil {
		t.Error("LoadFrom should fail with invalid JSON")
	}
}

// Test SaveTo/LoadFrom empty cache
func TestCacheSaveLoadIOEmpty(t *testing.T) {
	cache := NewCache()

	// Save empty cache
	var buf bytes.Buffer
	err := cache.SaveTo(&buf)
	if err != nil {
		t.Fatalf("SaveTo failed: %v", err)
	}

	// Load into new cache
	newCache := NewCache()
	err = newCache.LoadFrom(&buf)
	if err != nil {
		t.Fatalf("LoadFrom failed: %v", err)
	}

	// Verify cache is still empty
	if newCache.ItemCount() != 0 {
		t.Errorf("Expected empty cache, got %d items", newCache.ItemCount())
	}
}

// Test Persistence (file-based methods)
func TestCachePersistence(t *testing.T) {
	cache := NewCache()
	filePath := filepath.Join("testdata", "test_cache.json")

	// Clean up
	defer os.Remove(filePath)

	// Set up test data
	cache.Set("key1", "value1", 5*time.Second)
	cache.Set("key2", 42, 5*time.Second)
	cache.Set("key3", []string{"a", "b", "c"}, 5*time.Second)

	// Save to file
	err := cache.SaveToFile(filePath)
	if err != nil {
		t.Fatalf("SaveToFile failed: %v", err)
	}

	// Create new cache and load from file
	newCache := NewCache()
	err = newCache.LoadFromFile(filePath)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	// Verify loaded data
	value1, found1 := newCache.Get("key1")
	value2, found2 := newCache.Get("key2")
	_, found3 := newCache.Get("key3")

	if !found1 || value1 != "value1" {
		t.Error("key1 not loaded correctly")
	}
	if !found2 || value2 != float64(42) { // JSON unmarshaling converts numbers to float64
		t.Errorf("key2 not loaded correctly, got %v (%T)", value2, value2)
	}
	if !found3 {
		t.Error("key3 not found after loading")
	}
}

func TestCacheExport(t *testing.T) {
	cache := NewCache()

	cache.Set("key1", "value1", 5*time.Second)
	cache.Set("key2", "value2", 5*time.Second)

	exported := cache.Export()

	if len(exported) != 2 {
		t.Errorf("Expected 2 exported items, got %d", len(exported))
	}

	if exported["key1"] != "value1" || exported["key2"] != "value2" {
		t.Error("Exported values don't match")
	}
}

// Test Advanced TTL Management
func TestAdvancedTTL(t *testing.T) {
	cache := NewCache()

	// Test SetWithAbsoluteExpiry
	expireTime := time.Now().Add(2 * time.Second)
	cache.SetWithAbsoluteExpiry("abs_key", "abs_value", expireTime)

	value, found := cache.Get("abs_key")
	if !found || value != "abs_value" {
		t.Error("SetWithAbsoluteExpiry failed")
	}

	// Test GetTTL
	ttl, found := cache.GetTTL("abs_key")
	if !found {
		t.Error("GetTTL should find the key")
	}
	if ttl <= 0 || ttl > 2*time.Second {
		t.Errorf("Expected TTL around 2 seconds, got %v", ttl)
	}

	// Test ExtendTTL
	extended := cache.ExtendTTL("abs_key", 3*time.Second)
	if !extended {
		t.Error("ExtendTTL should succeed")
	}

	newTTL, found := cache.GetTTL("abs_key")
	if !found || newTTL <= ttl {
		t.Error("TTL should be extended")
	}

	// Test Refresh
	cache.Set("refresh_key", "refresh_value", 1*time.Second)
	time.Sleep(500 * time.Millisecond)

	refreshed := cache.Refresh("refresh_key")
	if !refreshed {
		t.Error("Refresh should succeed")
	}

	refreshedTTL, found := cache.GetTTL("refresh_key")
	if !found || refreshedTTL < 900*time.Millisecond {
		t.Error("Key should have refreshed TTL")
	}
}

// Test Memory Usage Monitoring
func TestMemoryMonitoring(t *testing.T) {
	cache := NewCacheWithOptions(0, 1024, nil) // 1KB limit

	initialUsage := cache.MemoryUsage()
	if initialUsage != 0 {
		t.Errorf("Expected 0 initial usage, got %d", initialUsage)
	}

	// Add some data
	cache.Set("key1", strings.Repeat("x", 100), 5*time.Second)
	usage1 := cache.MemoryUsage()
	if usage1 <= initialUsage {
		t.Error("Memory usage should increase after adding data")
	}

	// Test memory limit enforcement
	cache.SetMemoryLimit(500) // Reduce limit
	cache.Set("key2", strings.Repeat("y", 200), 5*time.Second)

	// Should not exceed limit significantly due to eviction
	finalUsage := cache.MemoryUsage()
	if finalUsage > 600 { // Allow some overhead
		t.Errorf("Memory usage %d exceeds expected limit", finalUsage)
	}
}

func TestItemCount(t *testing.T) {
	cache := NewCache()

	if cache.ItemCount() != 0 {
		t.Error("Expected 0 items in new cache")
	}

	cache.Set("key1", "value1", 5*time.Second)
	cache.Set("key2", "value2", 5*time.Second)

	if cache.ItemCount() != 2 {
		t.Errorf("Expected 2 items, got %d", cache.ItemCount())
	}

	cache.Delete("key1")
	if cache.ItemCount() != 1 {
		t.Errorf("Expected 1 item after deletion, got %d", cache.ItemCount())
	}
}

// Test Thread-Safe Iteration
func TestThreadSafeIteration(t *testing.T) {
	cache := NewCache()

	// Set up test data
	cache.Set("key1", "value1", 5*time.Second)
	cache.Set("key2", "value2", 5*time.Second)
	cache.Set("key3", "value3", 5*time.Second)

	// Test ForEach
	visited := make(map[string]any)
	cache.ForEach(func(key string, value any, expiration time.Time) bool {
		visited[key] = value
		return true // continue iteration
	})

	if len(visited) != 3 {
		t.Errorf("Expected 3 visited items, got %d", len(visited))
	}

	// Test early termination
	count := 0
	cache.ForEach(func(key string, value any, expiration time.Time) bool {
		count++
		return count < 2 // stop after 2 items
	})

	if count != 2 {
		t.Errorf("Expected early termination after 2 items, got %d", count)
	}

	// Test Snapshot
	snapshot := cache.Snapshot()
	if len(snapshot) != 3 {
		t.Errorf("Expected 3 items in snapshot, got %d", len(snapshot))
	}
}

// Test Namespace Support
func TestNamespaceSupport(t *testing.T) {
	cache := NewCache()

	// Test namespace creation
	userNS := cache.Namespace("users")
	productNS := cache.Namespace("products")

	// Set data in different namespaces
	userNS.Set("1", "John", 5*time.Second)
	userNS.Set("2", "Jane", 5*time.Second)
	productNS.Set("1", "Widget", 5*time.Second)
	productNS.Set("2", "Gadget", 5*time.Second)

	// Test namespace isolation
	userValue, userFound := userNS.Get("1")
	productValue, productFound := productNS.Get("1")

	if !userFound || userValue != "John" {
		t.Error("User namespace data not found")
	}
	if !productFound || productValue != "Widget" {
		t.Error("Product namespace data not found")
	}

	// Cross-namespace access should not work
	_, crossFound := userNS.Get("1") // This should only find users:1, not products:1
	if !crossFound {
		t.Error("Should find user:1 in user namespace")
	}

	// Test namespace keys
	userKeys := userNS.Keys()
	productKeys := productNS.Keys()

	if len(userKeys) != 2 || len(productKeys) != 2 {
		t.Errorf("Expected 2 keys in each namespace, got users:%d, products:%d", len(userKeys), len(productKeys))
	}

	// Test namespace item count
	if userNS.ItemCount() != 2 {
		t.Errorf("Expected 2 items in user namespace, got %d", userNS.ItemCount())
	}

	// Test ListNamespaces
	namespaces := cache.ListNamespaces()
	if len(namespaces) != 2 {
		t.Errorf("Expected 2 namespaces, got %d", len(namespaces))
	}

	// Test ClearNamespace
	clearedCount := cache.ClearNamespace("users")
	if clearedCount != 2 {
		t.Errorf("Expected to clear 2 items, cleared %d", clearedCount)
	}

	if userNS.ItemCount() != 0 {
		t.Error("User namespace should be empty after clear")
	}

	// Product namespace should be unaffected
	if productNS.ItemCount() != 2 {
		t.Error("Product namespace should still have 2 items")
	}
}

func TestNamespaceDelete(t *testing.T) {
	cache := NewCache()
	ns := cache.Namespace("test")

	ns.Set("key1", "value1", 5*time.Second)
	deleted := ns.Delete("key1")

	if !deleted {
		t.Error("Expected successful deletion")
	}

	_, found := ns.Get("key1")
	if found {
		t.Error("Key should be deleted")
	}
}

func TestNamespaceClear(t *testing.T) {
	cache := NewCache()
	ns := cache.Namespace("test")

	ns.Set("key1", "value1", 5*time.Second)
	ns.Set("key2", "value2", 5*time.Second)

	cleared := ns.Clear()
	if cleared != 2 {
		t.Errorf("Expected to clear 2 items, got %d", cleared)
	}

	if ns.ItemCount() != 0 {
		t.Error("Namespace should be empty after clear")
	}
}

// Test Cache Clear
func TestCacheClear(t *testing.T) {
	cache := NewCache()

	cache.Set("key1", "value1", 5*time.Second)
	cache.Set("key2", "value2", 5*time.Second)

	cache.Clear()

	if cache.ItemCount() != 0 {
		t.Error("Cache should be empty after clear")
	}

	if cache.MemoryUsage() != 0 {
		t.Error("Memory usage should be 0 after clear")
	}
}

// Test Health Check
func TestHealthCheck(t *testing.T) {
	cache := NewCacheWithOptions(100, 1024, nil)

	cache.Set("key1", "value1", 5*time.Second)
	cache.Get("key1")        // Hit
	cache.Get("nonexistent") // Miss

	health := cache.HealthCheck()

	// Check that all expected fields are present
	expectedFields := []string{"item_count", "memory_usage", "memory_limit", "max_size", "hit_ratio", "stats", "namespaces"}
	for _, field := range expectedFields {
		if _, exists := health[field]; !exists {
			t.Errorf("Health check missing field: %s", field)
		}
	}

	// Verify some values
	if health["item_count"] != 1 {
		t.Errorf("Expected item_count 1, got %v", health["item_count"])
	}

	if health["max_size"] != 100 {
		t.Errorf("Expected max_size 100, got %v", health["max_size"])
	}

	if health["memory_limit"] != int64(1024) {
		t.Errorf("Expected memory_limit 1024, got %v", health["memory_limit"])
	}

	// Hit ratio should be 0.5 (1 hit, 1 miss)
	hitRatio := health["hit_ratio"].(float64)
	if hitRatio < 0.4 || hitRatio > 0.6 {
		t.Errorf("Expected hit_ratio around 0.5, got %f", hitRatio)
	}
}

// Test Concurrent Access with New Features
func TestConcurrentAccessNewFeatures(t *testing.T) {
	cache := NewCacheWithOptions(1000, 0, nil)

	const numGoroutines = 10
	const numOperations = 100

	var wg sync.WaitGroup

	// Test concurrent namespace operations
	for i := range numGoroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ns := cache.Namespace(fmt.Sprintf("ns_%d", id))
			for j := range numOperations {
				key := fmt.Sprintf("key_%d", j)
				value := fmt.Sprintf("value_%d_%d", id, j)
				ns.Set(key, value, 5*time.Second)
			}
		}(i)
	}

	// Test concurrent pattern operations
	for i := range numGoroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for range numOperations {
				pattern := fmt.Sprintf("ns_%d:*", id%3)
				cache.GetByPattern(pattern)
			}
		}(i)
	}

	wg.Wait()

	// Verify final state
	if cache.ItemCount() != numGoroutines*numOperations {
		t.Errorf("Expected %d items, got %d", numGoroutines*numOperations, cache.ItemCount())
	}

	stats := cache.Stats()
	if stats.Sets != int64(numGoroutines*numOperations) {
		t.Errorf("Expected %d sets in stats, got %d", numGoroutines*numOperations, stats.Sets)
	}
}

// Test Edge Cases
func TestEdgeCases(t *testing.T) {
	cache := NewCache()

	// Test operations on non-existent keys
	if cache.ExtendTTL("nonexistent", time.Second) {
		t.Error("ExtendTTL should fail on non-existent key")
	}

	if cache.Refresh("nonexistent") {
		t.Error("Refresh should fail on non-existent key")
	}

	if _, found := cache.GetTTL("nonexistent"); found {
		t.Error("GetTTL should fail on non-existent key")
	}

	// Test empty pattern matches
	matches := cache.GetByPattern("nonmatching_*")
	if len(matches) != 0 {
		t.Error("Expected no matches for non-matching pattern")
	}

	// Test invalid regex
	matches = cache.GetByRegex("[invalid")
	if len(matches) != 0 {
		t.Error("Expected no matches for invalid regex")
	}

	// Test namespace operations on empty namespace
	ns := cache.Namespace("empty")
	if ns.ItemCount() != 0 {
		t.Error("Empty namespace should have 0 items")
	}

	keys := ns.Keys()
	if len(keys) != 0 {
		t.Error("Empty namespace should have no keys")
	}
}

// Benchmark new features
func BenchmarkCacheNamespace(b *testing.B) {
	cache := NewCache()
	ns := cache.Namespace("bench")

	for i := 0; b.Loop(); i++ {
		key := fmt.Sprintf("key_%d", i)
		ns.Set(key, "benchmark_value", 5*time.Second)
	}
}

func BenchmarkCacheGetByPattern(b *testing.B) {
	cache := NewCache()

	// Pre-populate cache
	for i := range 1000 {
		key := fmt.Sprintf("user_%d", i)
		cache.Set(key, "benchmark_value", 5*time.Second)
	}

	for b.Loop() {
		cache.GetByPattern("user_*")
	}
}

func BenchmarkCacheBatchOperations(b *testing.B) {
	cache := NewCache()

	items := make(map[string]any)
	for i := range 100 {
		items[fmt.Sprintf("key_%d", i)] = fmt.Sprintf("value_%d", i)
	}

	for b.Loop() {
		cache.SetMultiple(items, 5*time.Second)
	}
}

func BenchmarkCacheMemoryTracking(b *testing.B) {
	cache := NewCache()

	for i := 0; b.Loop(); i++ {
		key := fmt.Sprintf("key_%d", i)
		value := strings.Repeat("x", 100)
		cache.Set(key, value, 5*time.Second)
		cache.MemoryUsage()
	}
}
