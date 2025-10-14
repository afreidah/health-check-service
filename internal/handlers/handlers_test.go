package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/afreidah/health-check-service/internal/cache"
)

// TestHealthHandlerReturnsOK tests handler returns 200 when service is active
func TestHealthHandlerReturnsOK(t *testing.T) {
	// Setup cache with active status
	c := cache.New()
	c.UpdateStatus(http.StatusOK, "active")

	// Create test request
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	// Call handler
	HealthHandler(w, req, c)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// TestHealthHandlerReturnsServiceUnavailable tests 503 when service is inactive
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

// TestHealthHandlerReturnsInternalServerError tests 500 for errors
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

// TestHealthHandlerMultipleStatusCodes uses table-driven tests
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
			// Setup
			c := cache.New()
			c.UpdateStatus(tt.cacheStatus, tt.cacheState)

			req := httptest.NewRequest("GET", "/health", nil)
			w := httptest.NewRecorder()

			// Execute
			HealthHandler(w, req, c)

			// Assert
			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

// TestHealthHandlerWithDifferentHTTPMethods verifies handler works with various methods
func TestHealthHandlerWithDifferentHTTPMethods(t *testing.T) {
	methods := []string{"GET", "POST", "HEAD", "OPTIONS"}

	c := cache.New()
	c.UpdateStatus(http.StatusOK, "active")

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/health", nil)
			w := httptest.NewRecorder()

			HealthHandler(w, req, c)

			// Should return 200 regardless of method (no method checking in handler)
			if w.Code != http.StatusOK {
				t.Errorf("Method %s: expected status %d, got %d", method, http.StatusOK, w.Code)
			}
		})
	}
}

// TestHealthHandlerConcurrentRequests verifies thread safety
func TestHealthHandlerConcurrentRequests(t *testing.T) {
	c := cache.New()
	c.UpdateStatus(http.StatusOK, "active")

	// Fire off 100 concurrent requests
	done := make(chan bool, 100)

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

	// Wait for all to complete
	for i := 0; i < 100; i++ {
		<-done
	}
}
