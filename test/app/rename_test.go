package app_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunHelpUsesProjectName(t *testing.T) {
	var out, diagnostics bytes.Buffer
	code := runForTest(context.Background(), t, []string{"--help"}, &out, &diagnostics, "")
	assert.Zero(t, code)
	assert.Contains(t, out.String(), "netCatalog")
	assert.Empty(t, diagnostics.String())
}

func TestRunUsesIplistURLFlag(t *testing.T) {
	dir := t.TempDir()
	url := payloadServer(t, fullIPList, nil)
	result := execute(t, "update", "--source", "iplist", "--data-dir", dir, "--iplist-url", url)
	assert.Zero(t, result.code)
	assert.Empty(t, result.err)
	assert.Equal(t, commandResult{out: "iplist.example\n"}, execute(t, "get", "discord", "--type", "domains", "--data-dir", dir))
}
