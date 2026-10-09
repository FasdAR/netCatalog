package app_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunGetOutputDirWritesTypeFile(t *testing.T) {
	out := t.TempDir()
	r := executeCatalog(t, discordFixture, "get", "discord", "--type", "domain", "--output-dir", out)
	assert.Equal(t, commandResult{}, r)
	data, err := os.ReadFile(filepath.Join(out, "discord", "domain.md"))
	require.NoError(t, err)
	assert.Equal(t, "cdn.discord.com\ndiscord.gg\n", string(data))
}

func TestRunGetOutputDirKeepsOtherTypeFiles(t *testing.T) {
	out := t.TempDir()
	dir := fixtureDir(t, discordFixture, "")
	for _, kind := range []string{"domain", "ip"} {
		r := execute(t, "get", "discord", "--type", kind, "--output-dir", out, "--data-dir", dir)
		require.Zero(t, r.code)
	}
	assert.FileExists(t, filepath.Join(out, "discord", "domain.md"))
	data, err := os.ReadFile(filepath.Join(out, "discord", "ip.md"))
	require.NoError(t, err)
	assert.Equal(t, "192.0.2.1\n192.0.2.2\n", string(data))
}

func TestRunGetOutputDirNotCreatedOnFailure(t *testing.T) {
	out := filepath.Join(t.TempDir(), "results")
	r := executeCatalog(t, discordFixture, "get", "missing", "--type", "domain", "--output-dir", out)
	assert.Equal(t, 2, r.code)
	assert.NoDirExists(t, out)
}

func TestRunGetOutputDirUsesShortNameForSinglePortal(t *testing.T) {
	out := t.TempDir()
	r := executeCatalog(t, telegramFixture, "get", "telegram", "--type", "ip", "--output-dir", out)
	require.Zero(t, r.code)
	assert.FileExists(t, filepath.Join(out, "telegram", "ip.md"))
	assert.NoDirExists(t, filepath.Join(out, "telegram.org"))
}
