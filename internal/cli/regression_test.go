package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("output unavailable") }

func TestRunGetHelpWithoutServiceDoesNotRequestHTTP(t *testing.T) {
	var requests atomic.Int32
	server := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "must not be called", 503)
	})
	result := execute(t, "get", "--help", "--url", server.URL)
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
		http.Error(w, "must not be called", 503)
	})
	assertInvalid(t, "get", "youtube", "--type", "invalid", "--url", server.URL)
	assert.Zero(t, requests.Load())
}

func TestRunReturnsFailureWhenHelpCannotBeWritten(t *testing.T) {
	var diagnostics bytes.Buffer
	code := run(context.Background(), []string{"--help"}, failingWriter{}, &diagnostics, "")
	assert.Equal(t, 1, code)
	assert.Contains(t, diagnostics.String(), "output unavailable")
}

func TestRunReturnsFailureWhenSourcesCannotBeWritten(t *testing.T) {
	var diagnostics bytes.Buffer
	code := run(context.Background(), []string{"sources", "list"}, failingWriter{}, &diagnostics, "")
	assert.Equal(t, 1, code)
}

func TestRunReturnsFailureWhenEntriesCannotBeWritten(t *testing.T) {
	server := youtubeServer(t)
	var diagnostics bytes.Buffer
	code := run(context.Background(), []string{"get", "youtube", "--type", "domains", "--url", server.URL}, failingWriter{}, &diagnostics, "")
	assert.Equal(t, 1, code)
	assert.Contains(t, diagnostics.String(), "output unavailable")
}

func TestRunDoesNotRetainInfoFlag(t *testing.T) {
	server := warningServer(t)
	first := execute(t, "get", "telegram", "--type", "domains", "--info", "--url", server.URL)
	assert.Zero(t, first.code)
	assert.Contains(t, first.err, "warning:")
	second := execute(t, "get", "telegram", "--type", "domains", "--url", server.URL)
	assert.Equal(t, commandResult{out: "telegram.org\n"}, second)
}

func TestRunEmptyV2flyURLDisablesSource(t *testing.T) {
	var downloads atomic.Int32
	v2flyServer := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		downloads.Add(1)
		fmt.Fprint(w, release)
	})
	iplistServer := youtubeServer(t)
	var out, diagnostics bytes.Buffer
	code := run(context.Background(), []string{"get", "youtube", "--type", "domains", "--url", iplistServer.URL, "--v2fly-url", ""}, &out, &diagnostics, v2flyServer.URL)
	assert.Zero(t, code)
	assert.Equal(t, "youtube.com\n", out.String())
	assert.Empty(t, diagnostics.String())
	assert.Zero(t, downloads.Load())
}

func TestRunUnknownCommandReportsErrorOnce(t *testing.T) {
	result := execute(t, "unknown")
	assert.Equal(t, 2, result.code)
	assert.Contains(t, result.err, "unknown command")
	assert.NotContains(t, result.err, "Usage:")
	assert.Equal(t, 1, bytes.Count([]byte(result.err), []byte("unknown command")))
}
