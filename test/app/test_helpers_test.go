package app_test

import (
	"context"
	"io"
	"testing"

	"netCatalog/app"
)

func runForTest(ctx context.Context, t *testing.T, args []string, stdout, stderr io.Writer, defaultV2flyURL string) int {
	t.Helper()
	defaults := []string{"--data-dir", t.TempDir(), "--v2fly-url", defaultV2flyURL}
	return app.Run(ctx, append(defaults, args...), stdout, stderr)
}
