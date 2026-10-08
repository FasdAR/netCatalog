package cli

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunHelpUsesProjectName(t *testing.T) {
	var out, diagnostics bytes.Buffer
	code := run(context.Background(), []string{"--help"}, &out, &diagnostics, "")
	assert.Zero(t, code)
	assert.Contains(t, out.String(), "netCatalog")
	assert.Empty(t, diagnostics.String())
}
