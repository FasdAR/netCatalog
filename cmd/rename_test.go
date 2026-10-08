package cmd

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunHelpUsesProjectName(t *testing.T) {
	var out, diagnostics bytes.Buffer
	code := runForTest(t, context.Background(), []string{"--help"}, &out, &diagnostics, "")
	assert.Zero(t, code)
	assert.Contains(t, out.String(), "netCatalog")
	assert.Empty(t, diagnostics.String())
}

func TestRunUsesIplistURLFlag(t *testing.T) {
	server := youtubeServer(t)
	result := execute(t, "get", "youtube", "--type", "domains", "--iplist-url", server.URL)
	assert.Equal(t, commandResult{out: "youtube.com\n"}, result)
}
