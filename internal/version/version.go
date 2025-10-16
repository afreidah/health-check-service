// -----------------------------------------------------------------------------
// Version Information
// -----------------------------------------------------------------------------
//
// This package holds version metadata that is set at build time via ldflags.
// These variables are populated by the Makefile during compilation.
//
// Build Command (see Makefile):
//   go build \
//     -ldflags="-X github.com/afreidah/health-check-service/internal/version.Version=v1.0.0 \
//               -X github.com/afreidah/health-check-service/internal/version.Commit=abc123def \
//               -X github.com/afreidah/health-check-service/internal/version.BuildTime=2025-10-15T12:34:56Z"
//
// Usage in main.go:
//   fmt.Printf("health-checker %s\n", version.Version)
//   logger.Info("startup", "version", version.Version, "commit", version.Commit)
//
// Author: Alex Freidah <alex.freidah@gmail.com>
// License: Apache 2.0
// -----------------------------------------------------------------------------

package version

// Version is the semantic version of the application.
// Set at build time via ldflags: -X internal/version.Version=v1.2.3
var Version = "dev"

// Commit is the Git commit hash.
// Set at build time via ldflags: -X internal/version.Commit=abc123def
var Commit = "unknown"

// BuildTime is the timestamp when the binary was built.
// Set at build time via ldflags: -X internal/version.BuildTime=2025-10-15T12:34:56Z
var BuildTime = "unknown"

// String returns a formatted version string.
// Example: "v1.0.0 (commit: abc123def, built: 2025-10-15T12:34:56Z)"
func String() string {
	return Version + " (commit: " + Commit + ", built: " + BuildTime + ")"
}
