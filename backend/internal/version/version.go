package version

import "runtime"

var (
	Version   = "dev"
	BuildDate = ""
	GitCommit = ""
)

func Info() map[string]string {
	return map[string]string{
		"version":    Version,
		"build_date": BuildDate,
		"git_commit": GitCommit,
		"go_version": runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
	}
}
