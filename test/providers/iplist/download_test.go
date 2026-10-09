package iplist_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"netCatalog/internal/providers/iplist"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownloadFullExportClearsFilters(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assert.Equal(t, "json", r.URL.Query().Get("format"))
		assert.Len(t, r.URL.Query(), 2)
		assert.Equal(t, "token", r.URL.Query().Get("auth"))
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		writeResponse(t, w, snapshotFixture)
	}))
	t.Cleanup(s.Close)
	b, err := iplist.Download(context.Background(), s.URL+"?site=a&site[]=a&group=b&group[]=b&data=ip4&format=text&exclude[ip4]=x&exclude[site]=a&wildcard=1&native=1&domaintype=full&template=x&filesave=1&auth=token", s.Client())
	require.NoError(t, err)
	assert.JSONEq(t, snapshotFixture, string(b))
	assert.Equal(t, int32(1), calls.Load())
}

func TestDownloadRejectsHTTPFailure(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) }))
	t.Cleanup(s.Close)
	_, err := iplist.Download(context.Background(), s.URL, s.Client())
	require.ErrorContains(t, err, "503")
}

func TestDownloadRejectsMalformedSchema(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { writeResponse(t, w, `{"example.com":{}}`) }))
	t.Cleanup(s.Close)
	_, err := iplist.Download(context.Background(), s.URL, s.Client())
	require.Error(t, err)
}

func TestDownloadHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := iplist.Download(ctx, "https://example.test/", nil)
	require.ErrorIs(t, err, context.Canceled)
}

func TestDownloadReportsCloseFailure(t *testing.T) {
	closeErr := errors.New("close failed")
	_, err := iplist.Download(context.Background(), "https://example.test/", &http.Client{Transport: closeFailureTransport{err: closeErr}})
	require.ErrorIs(t, err, closeErr)
}

func TestDownloadRejectsOversizedExport(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { writeResponse(t, w, strings.Repeat(" ", (64<<20)+1)) }))
	t.Cleanup(s.Close)
	_, err := iplist.Download(context.Background(), s.URL, s.Client())
	require.ErrorContains(t, err, "exceeds")
}

func TestDownloadRejectsInvalidURL(t *testing.T) {
	_, err := iplist.Download(context.Background(), ":invalid", nil)
	require.Error(t, err)
}
