package iplist_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"netCatalog/internal/providers/iplist"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type closeFailureBody struct {
	io.Reader
	err error
}

func (b closeFailureBody) Close() error { return b.err }

type closeFailureTransport struct{ err error }

func (tr closeFailureTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       closeFailureBody{Reader: strings.NewReader(`{"example.com":{}}`), err: tr.err},
	}, nil
}

func TestListServicesReportsResponseCloseFailure(t *testing.T) {
	closeErr := errors.New("response close failed")
	p, err := iplist.New("https://example.test/", &http.Client{Transport: closeFailureTransport{err: closeErr}})
	require.NoError(t, err)
	_, err = p.ListServices(context.Background())
	require.ErrorIs(t, err, closeErr)
}
