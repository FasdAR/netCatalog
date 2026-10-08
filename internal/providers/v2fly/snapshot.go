package v2fly

import (
	"context"
	"fmt"

	"netCatalog/internal/resolver"
)

// SnapshotProvider exposes immutable compiled lists without network access.
type SnapshotProvider struct {
	lists map[string]list
}

// NewSnapshot parses and validates a bounded compiled YAML release.
func NewSnapshot(payload []byte) (*SnapshotProvider, error) {
	if len(payload) > maxBody {
		return nil, fmt.Errorf("release exceeds %d MiB", maxBody>>20)
	}
	lists, err := parseLists(payload)
	if err != nil {
		return nil, err
	}
	return &SnapshotProvider{lists: lists}, nil
}

// ID returns the source identifier.
func (*SnapshotProvider) ID() string { return "v2fly" }

// Capabilities returns Domains, the only supported kind.
func (*SnapshotProvider) Capabilities() []resolver.DataKind {
	return []resolver.DataKind{resolver.Domains}
}

// ListServices returns sorted, filtered named lists from the local snapshot.
func (p *SnapshotProvider) ListServices(ctx context.Context) ([]resolver.Service, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return listServices(p.lists), nil
}

// Fetch returns normalized domain rules for one local list.
func (p *SnapshotProvider) Fetch(ctx context.Context, key string, q resolver.Query) (resolver.Result, error) {
	return fetchList(ctx, p.lists, key, q)
}
