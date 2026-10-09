package iplist_test

import (
	"context"
	"netCatalog/internal/providers/iplist"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"netCatalog/internal/resolver"
)

const snapshotFixture = `{"telegram.org":{"group":"messengers","domains":["*.Example.org","bad domain"],"ip4":["192.0.2.1"],"ip6":["2001:db8::1"],"cidr4":["192.0.2.123/24"],"cidr6":["2001:db8::1/64"]}}`

func snapshotProvider(t *testing.T) *iplist.SnapshotProvider {
	t.Helper()
	p, err := iplist.NewSnapshot([]byte(snapshotFixture))
	require.NoError(t, err)
	return p
}

func TestSnapshotListServices(t *testing.T) {
	s, err := snapshotProvider(t).ListServices(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []resolver.Service{{Domain: "telegram.org", Bindings: []resolver.Binding{{Source: "iplist", Key: "telegram.org"}}}}, s)
}

func TestSnapshotFetchDomains(t *testing.T) {
	r, err := snapshotProvider(t).Fetch(context.Background(), "telegram.org", resolver.Query{Kind: resolver.Domains})
	require.NoError(t, err)
	assert.Equal(t, []resolver.Entry{{Value: "example.org", RuleType: "suffix"}}, r.Entries)
	assert.Equal(t, 1, r.Succeeded)
	assert.Len(t, r.Warnings, 1)
}

func TestSnapshotFetchIPsAll(t *testing.T) {
	r, err := snapshotProvider(t).Fetch(context.Background(), "telegram.org", resolver.Query{Kind: resolver.IPs, Family: resolver.All})
	require.NoError(t, err)
	assert.Equal(t, []resolver.Entry{{Value: "192.0.2.1"}, {Value: "2001:db8::1"}}, r.Entries)
	assert.Equal(t, 2, r.Succeeded)
}

func TestSnapshotFetchCIDRsAll(t *testing.T) {
	r, err := snapshotProvider(t).Fetch(context.Background(), "telegram.org", resolver.Query{Kind: resolver.CIDRs, Family: resolver.All})
	require.NoError(t, err)
	assert.Equal(t, []resolver.Entry{{Value: "192.0.2.0/24"}, {Value: "2001:db8::/64"}}, r.Entries)
	assert.Equal(t, 2, r.Succeeded)
	assert.Len(t, r.Warnings, 1)
}

func TestSnapshotFetchEmptyArrays(t *testing.T) {
	p, err := iplist.NewSnapshot([]byte(`{"example.org":{"domains":[],"ip4":[],"ip6":[],"cidr4":[],"cidr6":[]}}`))
	require.NoError(t, err)
	r, err := p.Fetch(context.Background(), "example.org", resolver.Query{Kind: resolver.IPs, Family: resolver.All})
	require.NoError(t, err)
	assert.Equal(t, 2, r.Succeeded)
	assert.Empty(t, r.Entries)
}

func TestSnapshotFetchUnknownKey(t *testing.T) {
	r, err := snapshotProvider(t).Fetch(context.Background(), "missing.org", resolver.Query{Kind: resolver.Domains})
	require.Error(t, err)
	assert.Zero(t, r.Succeeded)
}

func TestNewSnapshotRejectsMissingField(t *testing.T) {
	_, err := iplist.NewSnapshot([]byte(strings.Replace(snapshotFixture, `"ip6":["2001:db8::1"],`, "", 1)))
	require.Error(t, err)
}

func TestNewSnapshotRejectsWrongFieldType(t *testing.T) {
	_, err := iplist.NewSnapshot([]byte(strings.Replace(snapshotFixture, `"ip6":["2001:db8::1"]`, `"ip6":"bad"`, 1)))
	require.Error(t, err)
}

func TestNewSnapshotRejectsNullField(t *testing.T) {
	_, err := iplist.NewSnapshot([]byte(strings.Replace(snapshotFixture, `"ip6":["2001:db8::1"]`, `"ip6":null`, 1)))
	require.Error(t, err)
}

func TestNewSnapshotAcceptsPHPEmptyArray(t *testing.T) {
	p, err := iplist.NewSnapshot([]byte(`[]`))
	require.NoError(t, err)
	s, err := p.ListServices(context.Background())
	require.NoError(t, err)
	assert.Empty(t, s)
}

func TestSnapshotListServicesPreservesUpstreamKey(t *testing.T) {
	p, err := iplist.NewSnapshot([]byte(strings.Replace(snapshotFixture, "telegram.org", "TELEGRAM.ORG.", 1)))
	require.NoError(t, err)
	services, err := p.ListServices(context.Background())
	require.NoError(t, err)
	require.Len(t, services, 1)
	assert.Equal(t, "telegram.org", services[0].Domain)
	assert.Equal(t, "TELEGRAM.ORG.", services[0].Bindings[0].Key)
}

func TestSnapshotListServicesSkipsInvalidPortals(t *testing.T) {
	p, err := iplist.NewSnapshot([]byte(strings.Replace(snapshotFixture, `{"telegram.org"`, `{"bad domain":{},"other.org":[],"telegram.org"`, 1)))
	require.NoError(t, err)
	s, err := p.ListServices(context.Background())
	require.ErrorContains(t, err, "skipped 2")
	assert.Len(t, s, 1)
}

func TestSnapshotFetchHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := snapshotProvider(t).Fetch(ctx, "telegram.org", resolver.Query{Kind: resolver.Domains})
	require.ErrorIs(t, err, context.Canceled)
}

func TestSnapshotFetchRejectsInvalidFamily(t *testing.T) {
	_, err := snapshotProvider(t).Fetch(context.Background(), "telegram.org", resolver.Query{Kind: resolver.IPs, Family: "bad"})
	require.Error(t, err)
}

func TestSnapshotFetchRejectsInvalidKind(t *testing.T) {
	_, err := snapshotProvider(t).Fetch(context.Background(), "telegram.org", resolver.Query{Kind: "bad"})
	require.Error(t, err)
}
