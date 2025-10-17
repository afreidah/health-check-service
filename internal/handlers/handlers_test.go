// -----------------------------------------------------------------------
// HTTP Handlers - Tests
// -----------------------------------------------------------------------
//
// Package handlers_test validates health check endpoint handler behavior
// under various conditions. This endpoint is the primary interface for
// monitoring systems, making correct behavior critical for production
// reliability.
//
// Run with race detector: go test -race ./internal/handlers
//
// -----------------------------------------------------------------------

package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/afreidah/health-check-service/internal/cache"
)

// -----------------------------------------------------------------------
// Basic Status Code Tests
// -----------------------------------------------------------------------

// TestHealthHandlerReturnsOK verifies handler returns 200 when service is
// healthy (active).
func TestHealthHandlerReturnsOK(t *testing.T) {
	c := cache.New()
	c.UpdateStatus(http.StatusOK, "active")

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	HealthHandler(w, req, c)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// TestHealthHandlerReturnsServiceUnavailable verifies handler returns 503
// when the service is down. This tells monitoring systems the service
// should be taken out of rotation.
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
// when there is an error checking the service. This indicates a problem with
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

// -----------------------------------------------------------------------
// Table-Driven Tests
// -----------------------------------------------------------------------

// TestHealthHandlerMultipleStatusCodes verifies all possible status codes
// and service states using table-driven testing.
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
			c := cache.New()
			c.UpdateStatus(tt.cacheStatus, tt.cacheState)

			req := httptest.NewRequest("GET", "/health", nil)
			w := httptest.NewRecorder()

			HealthHandler(w, req, c)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

// -----------------------------------------------------------------------
// HTTP Method Tests
// -----------------------------------------------------------------------

// TestHealthHandlerWithDifferentHTTPMethods verifies the handler accepts
// GET and HEAD requests. These are the appropriate methods for health check
// endpoints per RFC 7231.
func TestHealthHandlerWithDifferentHTTPMethods(t *testing.T) {
	methods := []string{"GET", "HEAD"} // Only these are allowed

	c := cache.New()
	c.UpdateStatus(http.StatusOK, "active")

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/health", nil)
			w := httptest.NewRecorder()

			HealthHandler(w, req, c)

			if w.Code != http.StatusOK {
				t.Errorf("Method %s: expected status %d, got %d", method, http.StatusOK, w.Code)
			}
		})
	}
}

// TestHealthHandlerRejectsUnsupportedMethods verifies POST, OPTIONS, etc.
// are correctly rejected with 405 Method Not Allowed.
func TestHealthHandlerRejectsUnsupportedMethods(t *testing.T) {
	unsupportedMethods := []string{"POST", "PUT", "DELETE", "PATCH", "OPTIONS"}

	c := cache.New()
	c.UpdateStatus(http.StatusOK, "active")

	for _, method := range unsupportedMethods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/health", nil)
			w := httptest.NewRecorder()

			HealthHandler(w, req, c)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("Method %s: expected status %d, got %d",
					method, http.StatusMethodNotAllowed, w.Code)
			}

			// Verify Allow header is set
			if w.Header().Get("Allow") == "" {
				t.Errorf("Method %s: Allow header should be set", method)
			}
		})
	}
}

// -----------------------------------------------------------------------
// Concurrency Tests
// -----------------------------------------------------------------------

// TestHealthHandlerConcurrentRequests verifies the handler is thread-safe
// under high concurrent load. In production, many HTTP requests may hit the
// health endpoint simultaneously.
//
// Run with: go test -race ./internal/handlers
func TestHealthHandlerConcurrentRequests(t *testing.T) {
	c := cache.New()
	c.UpdateStatus(http.StatusOK, "active")

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

// -----------------------------------------------------------------------
// Stale Data Tests
// -----------------------------------------------------------------------

// TestHealthHandlerStaleDataWarning verifies that the handler adds a Warning
// header when the cached data is stale. The header includes the age of the
// data for operational visibility.
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

	// Check that warning contains the expected parts
	expectedPrefix := "199 - Stale health check data"
	if !strings.HasPrefix(warning, expectedPrefix) {
		t.Errorf("Expected Warning header to start with '%s', got '%s'", expectedPrefix, warning)
	}

	// Verify it includes age information
	if !strings.Contains(warning, "age:") {
		t.Errorf("Expected Warning header to include age, got '%s'", warning)
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
