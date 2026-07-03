package storage

// Storage is the contract for a key-value backend.
// The key is always a string (document ID, chunk ID, etc.).
// The value is always a string (JSON-encoded document, raw text, etc.).
type Storage interface {
	// Put writes a key-value pair. Overwrites any existing value.
	Put(key, value string) error

	// Get retrieves the value for a key.
	// Returns ("", false) if the key does not exist or has been deleted.
	Get(key string) (string, bool)

	// Delete removes a key. Subsequent Get returns ("", false).
	Delete(key string) error

	// Each calls fn for every live key-value pair.
	// Order is not guaranteed.
	Each(fn func(key, value string))

	// Size returns the number of live keys.
	Size() int

	// Close flushes and releases any held resources.
	Close() error
}
