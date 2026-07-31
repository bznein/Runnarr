package app

// These values are replaced with -ldflags for release and deployment builds.
// Local go run and go test builds intentionally keep the development defaults.
var (
	BuildVersion       = "dev"
	BuildCommit        = "unknown"
	BuildMigrationHash = "unknown"
)

type BuildInfo struct {
	Version       string `json:"version"`
	Commit        string `json:"commit"`
	MigrationHash string `json:"migrationHash"`
}

func CurrentBuildInfo() BuildInfo {
	return BuildInfo{
		Version:       BuildVersion,
		Commit:        BuildCommit,
		MigrationHash: BuildMigrationHash,
	}
}
