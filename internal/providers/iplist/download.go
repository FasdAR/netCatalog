package iplist

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Download retrieves and validates one complete iplist JSON export.
// Selection and exclusion parameters in rawURL are removed to avoid partial snapshots.
func Download(ctx context.Context, rawURL string, client *http.Client) (payload []byte, err error) {
	p, err := New(rawURL, client)
	if err != nil {
		return nil, err
	}
	u := *p.base
	q := u.Query()
	for _, key := range []string{"format", "data", "group", "group[]", "site", "site[]", "wildcard", "native", "domaintype", "template", "filesave"} {
		q.Del(key)
	}
	for key := range q {
		if strings.HasPrefix(key, "exclude[") {
			q.Del(key)
		}
	}
	q.Set("format", "json")
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
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
		return nil, fmt.Errorf("iplist export exceeds %d MiB", maxBody>>20)
	}
	if _, err := NewSnapshot(payload); err != nil {
		return nil, err
	}
	return payload, nil
}
