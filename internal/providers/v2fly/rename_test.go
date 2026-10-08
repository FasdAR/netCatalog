package v2fly

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListServicesSendsProjectUserAgent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "netCatalog/0.1", r.UserAgent())
		writeResponse(t, w, fixture)
	}))
	t.Cleanup(server.Close)
	p, err := New(server.URL, server.Client())
	require.NoError(t, err)
	_, err = p.ListServices(context.Background())
	require.NoError(t, err)
}
