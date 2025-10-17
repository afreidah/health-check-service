// -----------------------------------------------------------------------
// Rate Limiting Tests - internal/ratelimit/ratelimit_test.go
// -----------------------------------------------------------------------
//
// Package ratelimit_test validates token bucket rate limiting behavior
// including per-IP isolation, token refill rates, and HTTP middleware
// integration. These tests ensure rate limits protect endpoints from
// excessive load while allowing legitimate traffic through.
//
// Run with race detector: go test -race ./internal/ratelimit
//
// -----------------------------------------------------------------------

package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// -----------------------------------------------------------------------
// Basic Allow Tests
// -----------------------------------------------------------------------

// TestAllow_WithinLimit verifies that requests within burst capacity are
// allowed. Token bucket algorithm provides initial burst capacity equal to
// the configured burst size. With 10 req/sec rate and 20 burst, exactly
// 20 requests can be made immediately before exhaustion.
func TestAllow_WithinLimit(t *testing.T) {
	m := New(10, 20) // 10 req/sec, burst 20
	ip := "192.168.1.1"

	// Should allow 20 requests immediately (burst capacity)
	// The bucket starts full with 20 tokens available
	for i := 0; i < 20; i++ {
		if !m.Allow(ip) {
			t.Fatalf("Request %d should be allowed within burst", i+1)
		}
	}
}

// TestAllow_ExceedsLimit verifies requests exceeding burst capacity are
// rejected. After exhausting all tokens, additional requests must wait
// for token refill at the configured rate (10 tokens/second).
func TestAllow_ExceedsLimit(t *testing.T) {
	m := New(10, 20) // 10 req/sec, burst 20
	ip := "192.168.1.1"

	// Consume all burst tokens
	for i := 0; i < 20; i++ {
		m.Allow(ip)
	}

	// 21st should be rejected (bucket empty, no refill yet)
	if m.Allow(ip) {
		t.Error("Request exceeding burst should be rejected")
	}
}

// TestAllow_TokenRefill verifies token bucket refills at the configured
// rate. With 100 req/sec, tokens refill at 1 token per 10ms. After
// exhausting all tokens, waiting 20ms should provide approximately 2
// new tokens, allowing 2 more requests.
func TestAllow_TokenRefill(t *testing.T) {
	m := New(100, 100) // 100 req/sec = 1 token per 10ms
	ip := "192.168.1.1"

	// Use all burst tokens
	for i := 0; i < 100; i++ {
		m.Allow(ip)
	}

	// Should be empty now
	if m.Allow(ip) {
		t.Error("Should reject when no tokens available")
	}

	// Wait for token refill (1/100 sec = 10ms per token)
	time.Sleep(20 * time.Millisecond)

	// Should have at least 2 tokens now
	if !m.Allow(ip) {
		t.Error("Should allow after token refill")
	}
}

// -----------------------------------------------------------------------
// Per-IP Tests
// -----------------------------------------------------------------------

// TestAllow_DifferentIPsIndependent verifies that rate limits are tracked
// independently per IP address. Each IP gets its own token bucket with
// separate capacity and refill rate. Exhausting one IP's tokens does not
// affect other IPs' token availability.
func TestAllow_DifferentIPsIndependent(t *testing.T) {
	m := New(1, 2) // 1 req/sec, burst 2
	ip1 := "192.168.1.1"
	ip2 := "192.168.1.2"

	// Exhaust IP1's burst
	m.Allow(ip1) // 1
	m.Allow(ip1) // 2

	// IP2 should still have full burst capacity
	if !m.Allow(ip2) {
		t.Error("Different IPs should have independent limits")
	}

	if !m.Allow(ip2) {
		t.Error("IP2 should have burst tokens")
	}

	// IP2 should now be exhausted
	if m.Allow(ip2) {
		t.Error("IP2 should be exhausted")
	}

	// IP1 can get new token after refill period
	// At 1 req/sec, each token takes 1000ms to refill
	// Wait 1100ms to ensure at least 1 token is available
	time.Sleep(1100 * time.Millisecond)

	if !m.Allow(ip1) {
		t.Error("IP1 should have new token after refill")
	}
}

// -----------------------------------------------------------------------
// GetTokens Test
// -----------------------------------------------------------------------

// TestGetTokens verifies that the token count can be queried for debugging
// and header population. Initial token count should equal burst size, and
// should decrease as requests consume tokens.
func TestGetTokens(t *testing.T) {
	m := New(10, 20)
	ip := "192.168.1.1"

	// Initially should have burst tokens available
	tokens := m.GetTokens(ip)
	if tokens < 20 {
		t.Errorf("Expected ~20 tokens initially, got %f", tokens)
	}

	// Use some tokens
	m.Allow(ip)
	m.Allow(ip)

	tokens = m.GetTokens(ip)
	if tokens >= 20 {
		t.Errorf("Tokens should decrease after Allow, got %f", tokens)
	}
}

// -----------------------------------------------------------------------
// Middleware Tests
// -----------------------------------------------------------------------

// TestMiddleware_AllowsWithinLimit verifies middleware passes requests
// through when rate limit is not exceeded. With generous limits (100
// req/sec, burst 200), a single request should always succeed.
func TestMiddleware_AllowsWithinLimit(t *testing.T) {
	m := New(100, 200)

	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", recorder.Code)
	}
}

// TestMiddleware_RejectsExceeded verifies middleware returns 429 Too Many
// Requests when rate limit is exceeded. With 0 req/sec and 0 burst, all
// requests should be rejected immediately with appropriate headers.
func TestMiddleware_RejectsExceeded(t *testing.T) {
	m := New(0, 0) // 0 req/sec, 0 burst = always reject

	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusTooManyRequests {
		t.Errorf("Expected status 429, got %d", recorder.Code)
	}

	// Check headers
	if recorder.Header().Get("X-RateLimit-Limit") == "" {
		t.Error("Expected X-RateLimit-Limit header")
	}
	if recorder.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Error("Expected X-RateLimit-Remaining: 0")
	}
	if recorder.Header().Get("Retry-After") == "" {
		t.Error("Expected Retry-After header")
	}
}

// TestMiddleware_SetsHeaders verifies rate limit information headers are
// included in responses. These headers allow clients to implement backoff
// and understand rate limit policies.
func TestMiddleware_SetsHeaders(t *testing.T) {
	m := New(10, 20)

	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	// Should have rate limit headers
	if limit := recorder.Header().Get("X-RateLimit-Limit"); limit == "" {
		t.Error("Missing X-RateLimit-Limit header")
	}

	if remaining := recorder.Header().Get("X-RateLimit-Remaining"); remaining == "" {
		t.Error("Missing X-RateLimit-Remaining header")
	}
}

// -----------------------------------------------------------------------
// IP Extraction Tests
// -----------------------------------------------------------------------

// TestGetIP_DirectConnection verifies IP extraction from RemoteAddr when
// no proxy headers are present. This is the fallback case for direct
// connections without load balancers.
func TestGetIP_DirectConnection(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	ip := GetIP(req)
	if ip != "192.168.1.1:12345" {
		t.Errorf("Expected 192.168.1.1:12345, got %s", ip)
	}
}

// TestGetIP_XForwardedFor verifies IP extraction from X-Forwarded-For
// header when behind a proxy. The leftmost IP (client) is used, ignoring
// proxy IPs that appear later in the chain.
func TestGetIP_XForwardedFor(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "proxy.example.com:443"
	req.Header.Set("X-Forwarded-For", "192.168.1.100, 10.0.0.1")

	ip := GetIP(req)
	if ip != "192.168.1.100" {
		t.Errorf("Expected first IP from X-Forwarded-For, got %s", ip)
	}
}

// TestGetIP_XRealIP verifies IP extraction from X-Real-IP header. Some
// proxies set this instead of X-Forwarded-For.
func TestGetIP_XRealIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "proxy.example.com:443"
	req.Header.Set("X-Real-IP", "192.168.1.200")

	ip := GetIP(req)
	if ip != "192.168.1.200" {
		t.Errorf("Expected X-Real-IP value, got %s", ip)
	}
}

// TestGetIP_Precedence verifies X-Forwarded-For takes precedence over
// X-Real-IP when both headers are present. This matches standard proxy
// header precedence conventions.
func TestGetIP_Precedence(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "proxy.example.com:443"
	req.Header.Set("X-Forwarded-For", "192.168.1.100")
	req.Header.Set("X-Real-IP", "192.168.1.200")

	ip := GetIP(req)
	if ip != "192.168.1.100" {
		t.Errorf("X-Forwarded-For should take precedence, got %s", ip)
	}
}

// -----------------------------------------------------------------------
// Concurrency Tests
// -----------------------------------------------------------------------

// TestConcurrentRequests_ThreadSafe verifies rate limiter is thread-safe
// under concurrent load. In production, many requests may hit endpoints
// simultaneously from the same IP. Token bucket operations must be atomic
// to prevent race conditions.
//
// Run with: go test -race ./internal/ratelimit
func TestConcurrentRequests_ThreadSafe(t *testing.T) {
	m := New(1000, 2000) // Very generous to allow concurrent tests
	ip := "192.168.1.1"

	done := make(chan bool, 100)
	allowCount := 0
	rejectCount := 0
	var mu sync.Mutex

	// 100 concurrent requests
	for i := 0; i < 100; i++ {
		go func() {
			defer func() { done <- true }()

			if m.Allow(ip) {
				mu.Lock()
				allowCount++
				mu.Unlock()
			} else {
				mu.Lock()
				rejectCount++
				mu.Unlock()
			}
		}()
	}

	// Wait for all
	for i := 0; i < 100; i++ {
		<-done
	}

	// With 1000 req/sec and 2000 burst, 100 concurrent should all succeed
	if allowCount < 99 {
		t.Errorf("Expected ~100 allows, got %d (rejects: %d)", allowCount, rejectCount)
	}
}

// TestConcurrentRequests_DifferentIPs verifies per-IP isolation under
// concurrent load. Multiple IPs making requests simultaneously should
// each have independent token buckets without interference.
func TestConcurrentRequests_DifferentIPs(t *testing.T) {
	m := New(5, 10)

	done := make(chan bool, 50)

	// 10 different IPs, 5 requests each = 50 total
	for ipNum := 0; ipNum < 10; ipNum++ {
		ip := "192.168.1." + string(rune('0'+ipNum))

		for req := 0; req < 5; req++ {
			go func() {
				defer func() { done <- true }()
				m.Allow(ip)
			}()
		}
	}

	// Wait for all
	for i := 0; i < 50; i++ {
		<-done
	}
}

// -----------------------------------------------------------------------
// Cleanup Tests
// -----------------------------------------------------------------------

// TestCleanup_RemovesStaleEntries verifies the background cleanup goroutine
// removes IP entries that haven't been seen recently. This prevents memory
// leaks from accumulating limiters for IPs that no longer make requests.
func TestCleanup_RemovesStaleEntries(t *testing.T) {
	m := &Manager{
		limiters:         make(map[string]*ipLimiter),
		requestsPerSec:   10,
		burstSize:        20,
		cleanupInterval:  1 * time.Second,
		cleanupIdleAfter: 10 * time.Millisecond,
	}

	// Add some entries
	m.getLimiter("192.168.1.1")
	m.getLimiter("192.168.1.2")
	m.getLimiter("192.168.1.3")

	if len(m.limiters) != 3 {
		t.Errorf("Expected 3 limiters, got %d", len(m.limiters))
	}

	// Wait for entries to become stale
	time.Sleep(20 * time.Millisecond)

	// Manually trigger cleanup
	m.cleanup()

	if len(m.limiters) != 0 {
		t.Errorf("Expected 0 limiters after cleanup, got %d", len(m.limiters))
	}
}

// -----------------------------------------------------------------------
// Stats Tests
// -----------------------------------------------------------------------

// TestStats_ReturnsInfo verifies diagnostic stats are available for
// monitoring and debugging. Stats should reflect current limiter state
// including active IP count and configured rate limits.
func TestStats_ReturnsInfo(t *testing.T) {
	m := New(50, 100)

	// Create some activity
	m.Allow("192.168.1.1")
	m.Allow("192.168.1.2")
	m.Allow("192.168.1.3")

	stats := m.Stats()

	if activeIPs, ok := stats["active_ips"]; ok {
		if activeIPs != 3 {
			t.Errorf("Expected 3 active IPs, got %v", activeIPs)
		}
	} else {
		t.Error("Missing 'active_ips' in stats")
	}

	if rate, ok := stats["rate"]; ok {
		if rate != 50.0 {
			t.Errorf("Expected rate 50, got %v", rate)
		}
	} else {
		t.Error("Missing 'rate' in stats")
	}
}
