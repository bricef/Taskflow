// Package version provides the build version for all TaskFlow binaries.
// The Version variable is set at build time via ldflags:
//
//	go build -ldflags "-X github.com/bricef/taskflow/internal/version.Version=v1.0.0"
//
// If not set, it defaults to "dev".
package version

var Version = "dev"
