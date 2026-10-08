package v2fly

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"netCatalog/internal/resolver"
)

func snapshotProvider(t *testing.T) *SnapshotProvider {
	t.Helper()
	p, err := NewSnapshot([]byte(fixture))
	require.NoError(t, err)
	return p
}

func TestSnapshotListServicesFiltersLists(t *testing.T) {
	s, err := snapshotProvider(t).ListServices(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []resolver.Service{
		{Name: "discord", Bindings: []resolver.Binding{{Source: "v2fly", Key: "discord"}}},
		{Name: "telegram", Bindings: []resolver.Binding{{Source: "v2fly", Key: "telegram"}}},
	}, s)
}

func TestSnapshotFetchPreservesDomainRules(t *testing.T) {
	r, err := snapshotProvider(t).Fetch(context.Background(), "discord", resolver.Query{Kind: resolver.Domains})
	require.NoError(t, err)
	assert.Equal(t, []resolver.Entry{{Value: "discord.com", RuleType: "suffix"}, {Value: "discord.gg", RuleType: "suffix"}}, r.Entries)
	assert.Equal(t, 1, r.Succeeded)
	assert.Len(t, r.Warnings, 2)
}

func TestSnapshotFetchRejectsUnknownKey(t *testing.T) {
	_, err := snapshotProvider(t).Fetch(context.Background(), "missing", resolver.Query{Kind: resolver.Domains})
	require.Error(t, err)
}

func TestSnapshotFetchRejectsIPs(t *testing.T) {
	_, err := snapshotProvider(t).Fetch(context.Background(), "discord", resolver.Query{Kind: resolver.IPs})
	require.Error(t, err)
}

func TestNewSnapshotRejectsLengthMismatch(t *testing.T) {
	_, err := NewSnapshot([]byte(`lists: [{name: discord, length: 2, rules: ["domain:discord.com"]}]`))
	require.Error(t, err)
}

func TestNewSnapshotRejectsDuplicateNames(t *testing.T) {
	_, err := NewSnapshot([]byte(`lists: [{name: discord, length: 0, rules: []}, {name: discord, length: 0, rules: []}]`))
	require.Error(t, err)
}

func TestNewSnapshotRejectsMultipleDocuments(t *testing.T) {
	_, err := NewSnapshot([]byte("lists: []\n---\nlists: []"))
	require.Error(t, err)
}

func TestNewSnapshotRejectsMissingLists(t *testing.T) {
	_, err := NewSnapshot([]byte(`{}`))
	require.Error(t, err)
}

func TestSnapshotListServicesHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := snapshotProvider(t).ListServices(ctx)
	require.ErrorIs(t, err, context.Canceled)
}

func TestNewSnapshotAcceptsExplicitEmptyRules(t *testing.T) {
	p, err := NewSnapshot([]byte(`lists: [{name: discord, length: 0, rules: []}]`))
	require.NoError(t, err)
	r, err := p.Fetch(context.Background(), "discord", resolver.Query{Kind: resolver.Domains})
	require.NoError(t, err)
	assert.Equal(t, 1, r.Succeeded)
	assert.Empty(t, r.Entries)
}

func TestNewSnapshotRejectsExcludedLengthMismatch(t *testing.T) {
	_, err := NewSnapshot([]byte(`lists: [{name: categoryall, length: 999, rules: []}]`))
	require.Error(t, err)
}

func TestNewSnapshotRejectsDuplicateExcludedNames(t *testing.T) {
	_, err := NewSnapshot([]byte(`lists: [{name: categoryall, length: 0, rules: []}, {name: categoryall, length: 0, rules: []}]`))
	require.Error(t, err)
}
