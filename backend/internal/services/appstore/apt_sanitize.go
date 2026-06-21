package appstore

import (
	"fmt"
	"runtime"

	"github.com/luuuunet/owpanel/internal/platform"
)

// SanitizeBrokenAptRepos exposes apt repository cleanup for other packages.
func SanitizeBrokenAptRepos() {
	platform.SanitizeBrokenAptRepos()
}

// RunMariaDBStackInstall installs MariaDB via the stack fallback script (MySQL-compatible).
func RunMariaDBStackInstall() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("MariaDB stack install only supported on Linux")
	}
	platform.SanitizeBrokenAptRepos()
	if err := runStackFallback("mariadb"); err != nil {
		return err
	}
	return startMySQLService("mariadb")
}
