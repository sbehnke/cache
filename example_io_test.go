package cache

import (
	"bytes"
	"fmt"
	"strings"
	"time"
)

// ExampleCache_SaveTo demonstrates saving cache contents to any io.Writer
func ExampleCache_SaveTo() {
	cache := NewCache()

	// Add some data to the cache
	cache.Set("user:1", "John Doe", 5*time.Minute)
	cache.Set("user:2", "Jane Smith", 5*time.Minute)

	// Create a namespace and add some data
	sessionNS := cache.Namespace("sessions")
	sessionNS.Set("abc123", "active", 30*time.Minute)

	// Save to a bytes buffer (could be any io.Writer like a file, network connection, etc.)
	var buf bytes.Buffer
	err := cache.SaveTo(&buf)
	if err != nil {
		fmt.Printf("Error saving cache: %v\n", err)
		return
	}

	// Print the JSON output (truncated for example)
	output := buf.String()
	if len(output) > 100 {
		fmt.Printf("Saved cache data to buffer (approx %d bytes)\n", buf.Len()/100*100)
	} else {
		fmt.Printf("Cache data: %s", output)
	}

	// Output: Saved cache data to buffer (approx 500 bytes)
}

// ExampleCache_LoadFrom demonstrates loading cache contents from any io.Reader
func ExampleCache_LoadFrom() {
	// JSON data representing cached items (could come from any io.Reader)
	jsonData := `[
		{
			"key": "user:1",
			"value": "John Doe",
			"expiration": "2025-12-31T23:59:59Z",
			"namespace": "",
			"original_ttl": 300000000000
		},
		{
			"key": "sessions:abc123",
			"value": "active",
			"expiration": "2025-12-31T23:59:59Z",
			"namespace": "sessions",
			"original_ttl": 1800000000000
		}
	]`

	cache := NewCache()
	reader := strings.NewReader(jsonData)

	err := cache.LoadFrom(reader)
	if err != nil {
		fmt.Printf("Error loading cache: %v\n", err)
		return
	}

	// Access the loaded data
	user, found := cache.Get("user:1")
	if found {
		fmt.Printf("Loaded user: %s\n", user)
	}

	// Access namespaced data
	sessionNS := cache.Namespace("sessions")
	session, found := sessionNS.Get("abc123")
	if found {
		fmt.Printf("Loaded session: %s\n", session)
	}

	fmt.Printf("Total items loaded: %d\n", cache.ItemCount())

	// Output:
	// Loaded user: John Doe
	// Loaded session: active
	// Total items loaded: 2
}

// ExampleCache_roundtrip demonstrates a complete save/load cycle
func ExampleCache_roundtrip() {
	// Create and populate original cache
	originalCache := NewCache()
	originalCache.Set("config:timeout", 30, 1*time.Hour)
	originalCache.Set("config:retries", 3, 1*time.Hour)

	// Add some namespaced data
	userNS := originalCache.Namespace("users")
	userNS.Set("1", map[string]interface{}{"name": "Alice", "active": true}, 2*time.Hour)

	fmt.Printf("Original cache items: %d\n", originalCache.ItemCount())

	// Save to buffer
	var buf bytes.Buffer
	err := originalCache.SaveTo(&buf)
	if err != nil {
		fmt.Printf("Save error: %v\n", err)
		return
	}

	// Create new cache and load from buffer
	newCache := NewCache()
	err = newCache.LoadFrom(&buf)
	if err != nil {
		fmt.Printf("Load error: %v\n", err)
		return
	}

	fmt.Printf("New cache items: %d\n", newCache.ItemCount())

	// Verify data integrity
	timeout, found := newCache.Get("config:timeout")
	if found {
		fmt.Printf("Timeout config: %v\n", timeout)
	}

	newUserNS := newCache.Namespace("users")
	user, found := newUserNS.Get("1")
	if found {
		fmt.Printf("User data preserved: %v\n", user)
	}

	// Output:
	// Original cache items: 3
	// New cache items: 3
	// Timeout config: 30
	// User data preserved: map[active:true name:Alice]
}
