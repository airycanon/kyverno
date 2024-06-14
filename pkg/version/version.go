package version

import (
	"runtime/debug"

	"github.com/go-logr/logr"
)

// BuildVersion is provided by goreleaser througth the .goreleaser.yaml at compile-time
var BuildVersion string

func Version() string {
	if BuildVersion == "" {
		bi, ok := debug.ReadBuildInfo()
		if !ok {
			return "---"
		}
		BuildVersion = bi.Main.Version
	}
	return BuildVersion
}

// BuildTime is provided by goreleaser througth the .goreleaser.yaml at compile-time
var BuildTime string

func Time() string {
	bi, ok := debug.ReadBuildInfo()
	if ok {
		for _, setting := range bi.Settings {
			if setting.Key == "vcs.time" {
				return setting.Value
			}
		}
	}
	return BuildTime
}

// BuildHash is provided by goreleaser througth the .goreleaser.yaml at compile-time
var BuildHash string

func Hash() string {
	bi, ok := debug.ReadBuildInfo()
	if ok {
		for _, setting := range bi.Settings {
			if setting.Key == "vcs.revision" {
				return setting.Value
			}
		}
	}
	return BuildHash
}

// PrintVersionInfo displays the kyverno version - git version
func PrintVersionInfo(log logr.Logger) {
	log.Info("version", "version", Version(), "hash", Hash(), "time", Time())
}
