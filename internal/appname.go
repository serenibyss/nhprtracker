package internal

import (
	"fmt"
	"runtime"
	"strings"
)

const (
	AppName              = "nhprtracker"
	DefaultOrganization  = "GTNewHorizons"
	DefaultReleaseBranch = "release/2.7.x"
	DefaultStartDate     = "2024-12-08"
	DefaultFormatting    = "terminal"
)

// Set via LDFLAGS -X
var (
	Version = "unknown"
	Branch  = "unknown"
	Commit  = "unknown"

	// ExcludedRepositories are repos unversioned by DAXXL
	ExcludedRepositories = []string{
		"DreamAssemblerXXL",
		"GT-New-Horizons-Modpack",
		"GTNH-Translations",
		"RetroFuturaGradle",
		"GTNHGradle",
		"Twist-Space-Technology-Mod",
		"GTNH-Web-Map",
		"CustomGTCapeHook-Cape-List",
		"JustEnoughCalculation", // some day this may change
		"GTNHIssueHelper",
		"StructureLib",
		"worldedit-gtnh",
	}

	// ExcludedPRTitles are to catch PRs that don't need to be reported, like spotless formatting PRs
	ExcludedPRTitles = []string{
		"Spotless apply for branch",
	}
)

func AppVersion() string {
	return fmt.Sprintf(
		"%s version %s (git: %s@%s) (go: %s) (os: %s/%s)",
		AppName,
		Version,
		Branch,
		Commit,
		strings.TrimLeft(runtime.Version(), "go"),
		runtime.GOOS,
		runtime.GOARCH,
	)
}
