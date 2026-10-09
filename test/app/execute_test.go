package app_test

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"netCatalog/app"
)

func TestExecuteDoesNotRetainInvalidSource(t *testing.T) {
	oldArgs, oldOut, oldErr := os.Args, os.Stdout, os.Stderr
	t.Cleanup(func() { os.Args, os.Stdout, os.Stderr = oldArgs, oldOut, oldErr })
	invoke := func(wantCode int, args ...string) string {
		t.Helper()
		file, err := os.CreateTemp(t.TempDir(), "output")
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, file.Close()) })
		os.Args = append([]string{"netCatalog"}, args...)
		os.Stdout, os.Stderr = file, file
		require.Equal(t, wantCode, app.Execute(context.Background()))
		_, err = file.Seek(0, io.SeekStart)
		require.NoError(t, err)
		output, err := io.ReadAll(file)
		require.NoError(t, err)
		return string(output)
	}
	assert.Contains(t, invoke(2, "sources", "--source", "bad"), "iplist or v2fly")
	assert.Equal(t, "iplist\tdomains,ips,cidrs\t-\nv2fly\tdomains\t-\n", invoke(0, "sources", "--data-dir", t.TempDir()))
}
