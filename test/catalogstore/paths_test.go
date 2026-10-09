package catalogstore_test

import (
	"netCatalog/internal/catalogstore"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirForExecutableUsesSiblingSourceDir(t *testing.T) {
	got := catalogstore.DirForExecutable(filepath.Join("/opt", "netcatalog", "bin", "netCatalog"))
	assert.Equal(t, filepath.Join("/opt", "netcatalog", "bin", "source"), got)
}

func TestDefaultDirIsSourceNextToExecutable(t *testing.T) {
	got, err := catalogstore.DefaultDir()
	require.NoError(t, err)
	exe, err := os.Executable()
	require.NoError(t, err)
	exe, err = filepath.EvalSymlinks(exe)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(filepath.Dir(exe), "source"), got)
}
