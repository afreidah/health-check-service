// -----------------------------------------------------------------------------
// HTTP Handlers - Tests
// -----------------------------------------------------------------------------
//
// This test suite validates the health check endpoint handler behavior under
// various conditions. Since this endpoint is the primary interface for
// monitoring systems, correct behavior is critical for production reliability.
//
// Test Strategy:
//   - Use httptest for isolated handler testing (no real HTTP server needed)
//   - Test all possible HTTP status codes (200, 503, 500)
//   - Verify thread-safety under concurrent load
//   - Ensure handler works with any HTTP method
//
// Why These Tests Matter:
//   Incorrect status codes would cause monitoring systems to:
//     - Miss actual outages (false negatives)
//     - Trigger false alerts (false positives)
//     - Make incorrect routing/load balancing decisions
//
// Run with race detector: go test -race
//
// -----------------------------------------------------------------------------

package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/afreidah/health-check-service/internal/cache"
)

// -----------------------------------------------------------------------------
// Basic Status Code Tests
// -----------------------------------------------------------------------------

// TestHealthHandlerReturnsOK verifies handler returns 200 when service is
// healthy (active). This is the success case that monitoring systems look for.
func TestHealthHandlerReturnsOK(t *testing.T) {
	// Simulate background checker updating cache with healthy status
	c := cache.New()
	c.UpdateStatus(http.StatusOK, "active")

	// Create mock HTTP request
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	// Call handler
	HealthHandler(w, req, c)

	// Verify correct status code returned
	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// TestHealthHandlerReturnsServiceUnavailable verifies handler returns 503
// when the service is down. This tells monitoring systems the service is
// unhealthy and should be taken out of rotation.
func TestHealthHandlerReturnsServiceUnavailable(t *testing.T) {
	c := cache.New()
	c.UpdateStatus(http.StatusServiceUnavailable, "inactive")

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	HealthHandler(w, req, c)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

// TestHealthHandlerReturnsInternalServerError verifies handler returns 500
// when there's an error checking the service. This indicates a problem with
// the health checker itself, not the monitored service.
func TestHealthHandlerReturnsInternalServerError(t *testing.T) {
	c := cache.New()
	c.UpdateStatus(http.StatusInternalServerError, "error")

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	HealthHandler(w, req, c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

// -----------------------------------------------------------------------------
// Table-Driven Tests
// -----------------------------------------------------------------------------

// TestHealthHandlerMultipleStatusCodes uses table-driven testing to verify
// all possible status codes and service states in a single test. This ensures
// comprehensive coverage of the status mapping.
func TestHealthHandlerMultipleStatusCodes(t *testing.T) {
	tests := []struct {
		name           string
		cacheStatus    int
		cacheState     string
		expectedStatus int
	}{
		{"active service", http.StatusOK, "active", http.StatusOK},
		{"inactive service", http.StatusServiceUnavailable, "inactive", http.StatusServiceUnavailable},
		{"failed service", http.StatusServiceUnavailable, "failed", http.StatusServiceUnavailable},
		{"error state", http.StatusInternalServerError, "error", http.StatusInternalServerError},
		{"unknown state", http.StatusServiceUnavailable, "unknown", http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup cache with test state
			c := cache.New()
			c.UpdateStatus(tt.cacheStatus, tt.cacheState)

			req := httptest.NewRequest("GET", "/health", nil)
			w := httptest.NewRecorder()

			// Execute handler
			HealthHandler(w, req, c)

			// Assert expected status code
			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// HTTP Method Tests
// -----------------------------------------------------------------------------

// TestHealthHandlerWithDifferentHTTPMethods verifies the handler works with
// any HTTP method. While GET is standard for health checks, some monitoring
// systems or load balancers may use HEAD or OPTIONS.
func TestHealthHandlerWithDifferentHTTPMethods(t *testing.T) {
	methods := []string{"GET", "POST", "HEAD", "OPTIONS"}

	c := cache.New()
	c.UpdateStatus(http.StatusOK, "active")

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/health", nil)
			w := httptest.NewRecorder()

			HealthHandler(w, req, c)

			// Handler doesn't check method, so all should return 200
			if w.Code != http.StatusOK {
				t.Errorf("Method %s: expected status %d, got %d", method, http.StatusOK, w.Code)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Concurrency Tests
// -----------------------------------------------------------------------------

// TestHealthHandlerConcurrentRequests verifies the handler is thread-safe
// under high concurrent load. In production, many HTTP requests may hit the
// health endpoint simultaneously.
//
// This test simulates 100 concurrent requests, which would expose race
// conditions if the cache isn't properly protected by mutexes.
//
// Run with: go test -race to detect data races
func TestHealthHandlerConcurrentRequests(t *testing.T) {
	c := cache.New()
	c.UpdateStatus(http.StatusOK, "active")

	// Channel for synchronization
	done := make(chan bool, 100)

	// Fire off 100 concurrent HTTP requests
	for i := 0; i < 100; i++ {
		go func() {
			req := httptest.NewRequest("GET", "/health", nil)
			w := httptest.NewRecorder()

			HealthHandler(w, req, c)

			if w.Code != http.StatusOK {
				t.Errorf("Concurrent request failed with status %d", w.Code)
			}

			done <- true
		}()
	}

	// Wait for all requests to complete
	for i := 0; i < 100; i++ {
		<-done
	}
}

// TestHealthHandlerStaleDataWarning verifies that the handler adds a Warning
// header when the cached data is stale.
func TestHealthHandlerStaleDataWarning(t *testing.T) {
	c := cache.New()
	c.UpdateStatus(http.StatusOK, "active")

	// Set lastChecked to 35 seconds ago
	c.SetLastChecked(time.Now().Add(-35 * time.Second))

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	HealthHandler(w, req, c)

	warning := w.Header().Get("Warning")
	if warning == "" {
		t.Error("Expected Warning header for stale data, got none")
	}

	expectedWarning := "199 - Stale health check data"
	if warning != expectedWarning {
		t.Errorf("Expected Warning header '%s', got '%s'", expectedWarning, warning)
	}

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// TestHealthHandlerFreshDataNoWarning verifies that fresh data does NOT
// trigger a Warning header.
func TestHealthHandlerFreshDataNoWarning(t *testing.T) {
	c := cache.New()
	c.UpdateStatus(http.StatusOK, "active")

	// Check immediately - data should be fresh
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	HealthHandler(w, req, c)

	// Should NOT have Warning header
	warning := w.Header().Get("Warning")
	if warning != "" {
		t.Errorf("Expected no Warning header for fresh data, got '%s'", warning)
	}

	// Should return 200 OK
	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}
