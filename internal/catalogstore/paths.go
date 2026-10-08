package catalogstore

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// DefaultDir returns the platform's per-user data directory for netCatalog.
// It never falls back to the current working directory.
func DefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		// An explicit environment directory may not require a home directory.
		home = ""
	}
	return defaultDir(runtime.GOOS, home, os.Getenv("XDG_DATA_HOME"), os.Getenv("LOCALAPPDATA"))
}

func defaultDir(goos, home, xdg, local string) (string, error) {
	if goos == "linux" && filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "netCatalog"), nil
	}
	if goos == "windows" && local != "" {
		return filepath.Join(local, "netCatalog"), nil
	}
	if home == "" {
		return "", fmt.Errorf("unable to determine user data directory; specify --data-dir")
	}
	switch goos {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "netCatalog"), nil
	case "windows":
		return filepath.Join(home, "AppData", "Local", "netCatalog"), nil
	default:
		return filepath.Join(home, ".local", "share", "netCatalog"), nil
	}
}
