# Professional Go Documentation Style Guide

Keep it professional and concise. Document the API and non-obvious design decisions. Let clear code speak for itself.

---

## File Header Template

Every file starts with a box header block.

```go
// =============================================================================
// [One-Line Title]
// =============================================================================
//
// Package [name] [what it does].
// [Key behavior or design notes if non-obvious].
//
// [Brief config/usage notes if applicable]
//
// =============================================================================
```

### Good Example

```go
// =============================================================================
// Service Status Cache
// =============================================================================
//
// Package cache provides a thread-safe in-memory cache for service health status.
// The background checker writes periodically; HTTP handlers read concurrently.
// Cache continues serving last-known-good status on checker failure rather
// than returning unknown, allowing monitoring systems to remain operational.
//
// =============================================================================
```

### Bad Examples

```go
// BAD: No box header
package cache

// BAD: Box header but no package doc
// =============================================================================
// Cache Stuff
// =============================================================================
package cache

// BAD: Excessive documentation (write a book later, not here)
// =============================================================================
// Service Status Cache - A Comprehensive Guide
// =============================================================================
//
// This package provides... [pages of explanation]
```

---

## Type Documentation

```go
// TypeName [what it represents].
// [Important constraints or behavior if non-obvious].
type TypeName struct {
    field string
}
```

### Examples

```go
// ServiceCache holds the current health status. Fields are protected by
// RWMutex for thread-safe concurrent access by checker (writer) and
// HTTP handlers (readers).
type ServiceCache struct {
    mu sync.RWMutex
    // ...
}

// Options encapsulates logging configuration.
type Options struct {
    Level slog.Level
    // ...
}
```

---

## Function/Method Documentation

```go
// FunctionName [what it does].
// [Design decision or important caveat if non-obvious].
func FunctionName(param Type) Result
```

### Examples

```go
// UpdateStatus atomically updates cached status and transitions to StateRunning.
func (c *ServiceCache) UpdateStatus(code int, state string)

// IsStale checks if the last update is older than maxAge.
// Used to detect if the background checker has stopped responding.
func (c *ServiceCache) IsStale(maxAge time.Duration) bool

// CheckAndUpdateCacheWithReconnect performs a check and automatically
// reconnects on D-Bus failure using exponential backoff. Context is checked
// before waiting to allow graceful shutdown during reconnection attempts.
func CheckAndUpdateCacheWithReconnect(ctx context.Context, ...) *dbus.Conn
```

---

## Inline Comments

Comment the "why" for non-obvious code. Don't comment obvious things.

### Good

```go
// Use RLock to allow concurrent readers on read-only operations
mu.RLock()
defer mu.RUnlock()

// Check context before waiting to prevent shutdown from blocking on backoff timer
select {
case <-ctx.Done():
    return nil
default:
}

// Exponential backoff prevents overwhelming D-Bus during connection failures
retryDelay *= backoffMultiplier
```

### Bad

```go
mu.RLock()  // Acquire read lock
return statusCode  // Return status code
```

---

## Error Messages

Describe what failed, why it failed, and how to fix it.

### Good

```go
return fmt.Errorf(
    "invalid port: must be between 1-65535, got %d\n"+
    "use: --port 8080 or HEALTH_PORT=8080",
    cfg.Port)

return fmt.Errorf(
    "TLS certificate file not found: %s\n"+
    "ensure the file exists and is readable",
    c.TLSCertFile)
```

### Bad

```go
return fmt.Errorf("error")
return fmt.Errorf("invalid port")
```

---

## Test Documentation

State what's tested and why it matters.

```go
// TestFunctionName verifies that FunctionName correctly [does what].
// [Why failure would be bad].
func TestFunctionName(t *testing.T) {
```

### Good

```go
// TestStateToStatusCodeMapping verifies all systemd states map to correct
// HTTP codes. Incorrect mappings would cause monitoring systems to miss
// outages or trigger false alerts.
func TestStateToStatusCodeMapping(t *testing.T) {

// TestConcurrentAccess verifies thread safety under concurrent load.
// Run with: go test -race
func TestConcurrentAccess(t *testing.T) {
```

---

## Section Separators

Use box separators for major code sections within a file. Keep them minimal.

```go
// =============================================================================
// Type Definitions
// =============================================================================

// Code here

// =============================================================================
// Constructor
// =============================================================================

// Code here

// =============================================================================
// Public Methods
// =============================================================================

// Code here

// =============================================================================
// Helper Functions
// =============================================================================

// Code here
```

---

## Code Examples

Provide examples only for non-obvious usage.

### Good (necessary)

```go
// InitFromEnv creates a logger configured from environment variables.
// Environment variables: LOG_LEVEL, LOG_FORMAT, LOG_TAGS, LOG_SOURCE
//
// Example:
//   logger := logging.InitFromEnv(map[string]string{
//       "service": "health-check-service",
//       "version": "v1.2.3",
//   })
func InitFromEnv(extraTags map[string]string) *slog.Logger
```

### Bad (obvious API, don't comment)

```go
// GetStatus returns the status code and state.
//
// Example:
//   code, state := cache.GetStatus()
func (c *ServiceCache) GetStatus() (int, string)
```

---

## Checklist

Before committing:

- [ ] File has box header with package documentation
- [ ] Exported types are documented
- [ ] Exported functions are documented
- [ ] Complex "why" decisions are explained
- [ ] Error messages are actionable
- [ ] Tests explain what and why they test
- [ ] Inline comments explain "why", not "what"
- [ ] Examples provided only for non-obvious usage
- [ ] No numbered lists or conversational tone
- [ ] Documentation is proportionate to code complexity

---

## TL;DR

1. Every file starts with a box header
2. Brief package documentation (2-4 sentences max)
3. Document what exported things do and why if non-obvious
4. Inline comments for complex logic
5. Keep it professional and concise
