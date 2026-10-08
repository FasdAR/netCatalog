package v2fly

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// Download retrieves and validates a complete compiled YAML release.
// It returns the original bytes, including lists excluded from user-facing searches.
func Download(ctx context.Context, rawURL string, client *http.Client) (payload []byte, err error) {
	p, err := New(rawURL, client)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/yaml")
	req.Header.Set("User-Agent", "netCatalog/0.1")
	response, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, response.Body.Close()) }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	payload, err = io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	if err != nil {
		return nil, err
	}
	if len(payload) > maxBody {
		return nil, fmt.Errorf("release exceeds %d MiB", maxBody>>20)
	}
	if _, err := NewSnapshot(payload); err != nil {
		return nil, err
	}
	return payload, nil
}
