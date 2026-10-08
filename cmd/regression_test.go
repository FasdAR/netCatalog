package cmd

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("output unavailable") }

func TestRunGetHelpWithoutServiceDoesNotRequestHTTP(t *testing.T) {
	var requests atomic.Int32
	server := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "must not be called", http.StatusServiceUnavailable)
	})
	result := execute(t, "get", "--help", "--iplist-url", server.URL)
	assert.Zero(t, result.code)
	assert.Contains(t, result.out, "--type")
	assert.Empty(t, result.err)
	assert.Zero(t, requests.Load())
}

func TestRunSourcesRejectsExtraArgument(t *testing.T) {
	assertInvalid(t, "sources", "list", "extra")
}

func TestRunInvalidTypeDoesNotRequestHTTP(t *testing.T) {
	var requests atomic.Int32
	server := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "must not be called", http.StatusServiceUnavailable)
	})
	assertInvalid(t, "get", "youtube", "--type", "invalid", "--iplist-url", server.URL)
	assert.Zero(t, requests.Load())
}

func TestRunReturnsFailureWhenHelpCannotBeWritten(t *testing.T) {
	var diagnostics bytes.Buffer
	code := runForTest(t, context.Background(), []string{"--help"}, failingWriter{}, &diagnostics, "")
	assert.Equal(t, 1, code)
	assert.Contains(t, diagnostics.String(), "output unavailable")
}

func TestRunReturnsFailureWhenSourcesCannotBeWritten(t *testing.T) {
	var diagnostics bytes.Buffer
	code := runForTest(t, context.Background(), []string{"sources", "list"}, failingWriter{}, &diagnostics, "")
	assert.Equal(t, 1, code)
}

func TestRunReturnsFailureWhenEntriesCannotBeWritten(t *testing.T) {
	dir := fixtureDir(t, youtubeFixture, "")
	var diagnostics bytes.Buffer
	code := runForTest(t, context.Background(), []string{"get", "youtube", "--type", "domains", "--data-dir", dir}, failingWriter{}, &diagnostics, "")
	assert.Equal(t, 1, code)
	assert.Contains(t, diagnostics.String(), "output unavailable")
}

func TestRunDoesNotRetainInfoFlag(t *testing.T) {
	dir := fixtureDir(t, warningFixture, "")
	first := execute(t, "get", "telegram", "--type", "domains", "--info", "--data-dir", dir)
	assert.Zero(t, first.code)
	assert.Contains(t, first.err, "warning:")
	second := execute(t, "get", "telegram", "--type", "domains", "--data-dir", dir)
	assert.Equal(t, commandResult{out: "telegram.org\n"}, second)
}

func TestRunEmptyV2flyURLDoesNotDisableLocalSource(t *testing.T) {
	dir := fixtureDir(t, sharedFixture, release)
	tr := forbidHTTP(t)
	r := execute(t, "get", "discord", "--type", "domains", "--data-dir", dir, "--v2fly-url", "")
	assert.Equal(t, commandResult{out: "cdn.discord.com\nshared.com\n"}, r)
	assert.Zero(t, tr.calls.Load())
}

func TestRunUnknownCommandReportsErrorOnce(t *testing.T) {
	result := execute(t, "unknown")
	assert.Equal(t, 2, result.code)
	assert.Contains(t, result.err, "unknown command")
	assert.NotContains(t, result.err, "Usage:")
	assert.Equal(t, 1, bytes.Count([]byte(result.err), []byte("unknown command")))
}

func TestRunDoesNotRetainFamilyFlag(t *testing.T) {
	dir := fixtureDir(t, youtubeFixture, "")
	first := execute(t, "get", "youtube.com", "--site", "--type", "ips", "--family", "ipv6", "--data-dir", dir)
	assert.Equal(t, commandResult{}, first)
	second := execute(t, "get", "youtube.com", "--site", "--type", "domains", "--data-dir", dir)
	assert.Equal(t, commandResult{out: "youtube.com\n"}, second)
}

func TestRunDoesNotRetainSiteFlag(t *testing.T) {
	dir := fixtureDir(t, youtubeFixture, "")
	first := execute(t, "get", "youtube.com", "--site", "--type", "domains", "--data-dir", dir)
	assert.Equal(t, commandResult{out: "youtube.com\n"}, first)
	second := execute(t, "get", "youtube", "--type", "domains", "--data-dir", dir)
	assert.Equal(t, commandResult{out: "youtube.com\n"}, second)
}

func TestRunDoesNotRetainCancelledContext(t *testing.T) {
	dir := fixtureDir(t, youtubeFixture, "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, diagnostics bytes.Buffer
	code := runForTest(t, ctx, []string{"services", "list", "--data-dir", dir}, &out, &diagnostics, "")
	assert.Equal(t, 1, code)
	second := execute(t, "services", "list", "--data-dir", dir)
	assert.Equal(t, commandResult{out: "openai.com\nyoutube.com\n"}, second)
}

func TestRunDoesNotRetainFailedOutput(t *testing.T) {
	var diagnostics bytes.Buffer
	code := runForTest(t, context.Background(), []string{"--help"}, failingWriter{}, &diagnostics, "")
	assert.Equal(t, 1, code)
	second := execute(t, "--help")
	assert.Zero(t, second.code)
	assert.Contains(t, second.out, "netCatalog")
	assert.Empty(t, second.err)
}

func TestRootRegistersNestedListCommands(t *testing.T) {
	services, remaining, err := rootCmd.Find([]string{"services", "list"})
	require.NoError(t, err)
	assert.Empty(t, remaining)
	assert.Same(t, servicesListCmd, services)
	assert.Same(t, servicesCmd, services.Parent())
	sources, remaining, err := rootCmd.Find([]string{"sources", "list"})
	require.NoError(t, err)
	assert.Empty(t, remaining)
	assert.Same(t, sourcesListCmd, sources)
	assert.Same(t, sourcesCmd, sources.Parent())
}
