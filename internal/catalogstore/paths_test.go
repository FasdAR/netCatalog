package catalogstore

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultDirUsesLinuxXDG(t *testing.T) {
	got, err := defaultDir("linux", "/home/test", "/data", "")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/data", "netCatalog"), got)
}

func TestDefaultDirUsesLinuxFallback(t *testing.T) {
	got, err := defaultDir("linux", "/home/test", "", "")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/home/test", ".local", "share", "netCatalog"), got)
}

func TestDefaultDirIgnoresRelativeXDG(t *testing.T) {
	got, err := defaultDir("linux", "/home/test", "relative", "")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/home/test", ".local", "share", "netCatalog"), got)
}

func TestDefaultDirUsesMacOS(t *testing.T) {
	got, err := defaultDir("darwin", "/Users/test", "", "")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/Users/test", "Library", "Application Support", "netCatalog"), got)
}

func TestDefaultDirUsesWindowsLocal(t *testing.T) {
	got, err := defaultDir("windows", "/home/test", "", "/local")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/local", "netCatalog"), got)
}

func TestDefaultDirUsesWindowsFallback(t *testing.T) {
	got, err := defaultDir("windows", "/home/test", "", "")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/home/test", "AppData", "Local", "netCatalog"), got)
}

func TestDefaultDirRejectsMissingHome(t *testing.T) {
	_, err := defaultDir("darwin", "", "", "")
	require.Error(t, err)
}

func TestDefaultDirReadsEnvironment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("LOCALAPPDATA", "")
	got, err := DefaultDir()
	require.NoError(t, err)
	h, err := os.UserHomeDir()
	require.NoError(t, err)
	want, err := defaultDir(runtime.GOOS, h, "", "")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}
