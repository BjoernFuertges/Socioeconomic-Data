// Package version hält die Release-Version und den Build-Zeitpunkt der API.
//
// Die Werte werden beim Build per -ldflags gesetzt (siehe Dockerfile, in CI aus dem
// Semantic-Release-Ergebnis) und erscheinen in der OpenAPI-Spec (info.version), in der
// Swagger-UI und in der Antwort von GET /health. Lokale Builds ohne ldflags melden "dev".
package version

var (
	// Version ist die Release-Version, z. B. "1.4.2" oder "1.4.2+sha.abc1234" bei Builds ohne neuen Release.
	Version = "dev"
	// BuildTime ist der Build-Zeitpunkt als UTC-Zeitstempel (RFC 3339).
	BuildTime = "unknown"
)
