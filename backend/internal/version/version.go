// Package version holds the release version and build time of the API.
//
// The values are set at build time via -ldflags (see Dockerfile; in CI from the
// semantic-release result) and appear in the OpenAPI spec (info.version), in the
// Swagger UI and in the response of GET /health. Local builds without ldflags report "dev".
package version

var (
	// Version is the release version, e.g. "1.4.2" or "1.4.2+sha.abc1234" for builds without a new release.
	Version = "dev"
	// BuildTime is the build time as a UTC timestamp (RFC 3339).
	BuildTime = "unknown"
)
