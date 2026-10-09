package catalogstore

import (
	"fmt"
	"os"
	"path/filepath"
)

// DefaultDir returns the default catalog directory: the "source" folder next
// to the running executable. Symlinks to the executable are resolved. It never
// falls back to the current working directory.
func DefaultDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("unable to determine executable location; specify --data-dir: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return DirForExecutable(exe), nil
}

// DirForExecutable returns the catalog directory for an executable path:
// the "source" folder in the directory containing the executable.
func DirForExecutable(exe string) string {
	return filepath.Join(filepath.Dir(exe), "source")
}
