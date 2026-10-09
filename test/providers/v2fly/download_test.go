package v2fly_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"netCatalog/internal/providers/v2fly"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownloadPreservesFullRelease(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, http.MethodGet, r.Method)
		writeResponse(t, w, fixture)
	}))
	t.Cleanup(s.Close)
	b, err := v2fly.Download(context.Background(), s.URL, s.Client())
	require.NoError(t, err)
	assert.Equal(t, fixture, string(b))
	assert.Equal(t, int32(1), calls.Load())
}

func TestDownloadFollowsReleaseRedirect(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/latest" {
			http.Redirect(w, r, "/release", http.StatusFound)
			return
		}
		writeResponse(t, w, fixture)
	}))
	t.Cleanup(s.Close)
	b, err := v2fly.Download(context.Background(), s.URL+"/latest", s.Client())
	require.NoError(t, err)
	assert.Equal(t, fixture, string(b))
}

func TestDownloadRejectsInvalidRelease(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { writeResponse(t, w, "{}") }))
	t.Cleanup(s.Close)
	_, err := v2fly.Download(context.Background(), s.URL, s.Client())
	require.Error(t, err)
}

func TestDownloadRejectsOversizedRelease(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { writeResponse(t, w, strings.Repeat(" ", (32<<20)+1)) }))
	t.Cleanup(s.Close)
	_, err := v2fly.Download(context.Background(), s.URL, s.Client())
	require.ErrorContains(t, err, "exceeds")
}

func TestDownloadReportsCloseFailure(t *testing.T) {
	closeErr := errors.New("close failed")
	_, err := v2fly.Download(context.Background(), "https://example.test/", &http.Client{Transport: closeFailureTransport{err: closeErr}})
	require.ErrorIs(t, err, closeErr)
}

func TestDownloadHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := v2fly.Download(ctx, "https://example.test/", nil)
	require.ErrorIs(t, err, context.Canceled)
}

func TestDownloadRejectsHTTPFailure(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) }))
	t.Cleanup(s.Close)
	_, err := v2fly.Download(context.Background(), s.URL, s.Client())
	require.ErrorContains(t, err, "503")
}

func TestDownloadRejectsInvalidURL(t *testing.T) {
	_, err := v2fly.Download(context.Background(), ":invalid", nil)
	require.Error(t, err)
}
