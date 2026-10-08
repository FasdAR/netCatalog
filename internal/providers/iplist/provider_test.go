package iplist

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"netCatalog/internal/resolver"
)

func newTestProvider(t *testing.T, handler http.HandlerFunc) *Provider {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	p, err := New(server.URL, server.Client())
	require.NoError(t, err)
	return p
}

func responseProvider(t *testing.T, body string) *Provider {
	t.Helper()
	return newTestProvider(t, func(w http.ResponseWriter, _ *http.Request) { writeResponse(t, w, body) })
}

func writeResponse(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	_, err := fmt.Fprint(w, body)
	assert.NoError(t, err)
}

func TestFetchRetainsIPv4WhenIPv6Fails(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("data") == "ip6" {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		writeResponse(t, w, `{"youtube.com":["1.2.3.4"]}`)
	})
	result, err := p.Fetch(context.Background(), "youtube.com", resolver.Query{Kind: resolver.IPs, Family: resolver.All})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Succeeded)
	assert.Equal(t, []resolver.Entry{{Value: "1.2.3.4"}}, result.Entries)
	require.Len(t, result.Warnings, 1)
	assert.Contains(t, result.Warnings[0], "503")
}

func TestFetchDownloadsOnEveryCall(t *testing.T) {
	var calls atomic.Int32
	p := newTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeResponse(t, w, `{"youtube.com":["1.2.3.4"]}`)
	})
	_, err := p.Fetch(context.Background(), "youtube.com", resolver.Query{Kind: resolver.IPs, Family: resolver.IPv4})
	require.NoError(t, err)
	_, err = p.Fetch(context.Background(), "youtube.com", resolver.Query{Kind: resolver.IPs, Family: resolver.IPv4})
	require.NoError(t, err)
	assert.Equal(t, int32(2), calls.Load())
}

func TestFetchSkipsInvalidRecords(t *testing.T) {
	p := responseProvider(t, `{"youtube.com":["1.2.3.4","invalid","::1","1.2.3.4"],"other.com":["5.6.7.8"]}`)
	result, err := p.Fetch(context.Background(), "youtube.com", resolver.Query{Kind: resolver.IPs, Family: resolver.IPv4})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Succeeded)
	assert.Len(t, result.Warnings, 1)
	assert.ElementsMatch(t, []resolver.Entry{{Value: "1.2.3.4"}, {Value: "1.2.3.4"}, {Value: "5.6.7.8"}}, result.Entries)
}

func TestFetchOverridesStaleURLSelection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "youtube.com", r.URL.Query().Get("site"))
		assert.Equal(t, "ip4", r.URL.Query().Get("data"))
		assert.Equal(t, "json", r.URL.Query().Get("format"))
		assert.Empty(t, r.URL.Query().Get("group"))
		assert.Empty(t, r.URL.Query().Get("site[]"))
		assert.Empty(t, r.URL.Query().Get("group[]"))
		writeResponse(t, w, `{}`)
	}))
	t.Cleanup(server.Close)
	p, err := New(server.URL+"?site=stale&data=domains&group=stale&site[]=stale&group[]=stale", server.Client())
	require.NoError(t, err)
	_, err = p.Fetch(context.Background(), "youtube.com", resolver.Query{Kind: resolver.IPs, Family: resolver.IPv4})
	require.NoError(t, err)
}

func TestListServicesDiscoversSortedPortals(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.URL.Query().Get("data"))
		assert.Empty(t, r.URL.Query().Get("group"))
		writeResponse(t, w, `{"b.com":{"group":"youtube"},"a.com":{"group":"youtube"},"telegram.org":{"group":"messengers"},"c.com":{}}`)
	})
	services, err := p.ListServices(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []resolver.Service{
		{Domain: "a.com", Bindings: []resolver.Binding{{Source: "iplist", Key: "a.com"}}},
		{Domain: "b.com", Bindings: []resolver.Binding{{Source: "iplist", Key: "b.com"}}},
		{Domain: "c.com", Bindings: []resolver.Binding{{Source: "iplist", Key: "c.com"}}},
		{Domain: "telegram.org", Bindings: []resolver.Binding{{Source: "iplist", Key: "telegram.org"}}},
	}, services)
}

func TestListServicesKeepsValidPortalsOnPartialFailure(t *testing.T) {
	p := responseProvider(t, `{"TELEGRAM.ORG.":{"group":"messengers"},"bad domain":{},"error":"unavailable"}`)
	services, err := p.ListServices(context.Background())
	require.Error(t, err)
	assert.Equal(t, []resolver.Service{{Domain: "telegram.org", Bindings: []resolver.Binding{{Source: "iplist", Key: "TELEGRAM.ORG."}}}}, services)
}

func TestNormalizeWildcardDomain(t *testing.T) {
	entry, err := normalize("*.EXAMPLE.COM.", "domains")
	require.NoError(t, err)
	assert.Equal(t, resolver.Entry{Value: "example.com", RuleType: "suffix"}, entry)
}

func TestNormalizeRejectsDomainWhitespace(t *testing.T) {
	_, err := normalize("bad domain.com", "domains")
	assert.Error(t, err)
}

func TestNormalizeRejectsLeadingHyphen(t *testing.T) {
	_, err := normalize("-bad.com", "domains")
	assert.Error(t, err)
}

func TestNormalizeRejectsIPAsDomain(t *testing.T) {
	_, err := normalize("1.2.3.4", "domains")
	assert.Error(t, err)
}

func TestNormalizeRejectsRegexpAsDomain(t *testing.T) {
	_, err := normalize("regexp:test", "domains")
	assert.Error(t, err)
}

func TestNormalizeAcceptsPunycode(t *testing.T) {
	entry, err := normalize("xn--e1afmkfd.xn--p1ai", "domains")
	require.NoError(t, err)
	assert.Equal(t, "xn--e1afmkfd.xn--p1ai", entry.Value)
}

func TestNormalizeIPv4(t *testing.T) {
	entry, err := normalize("1.2.3.4", "ip4")
	require.NoError(t, err)
	assert.Equal(t, "1.2.3.4", entry.Value)
}

func TestNormalizeIPv6(t *testing.T) {
	entry, err := normalize("2001:0db8::1", "ip6")
	require.NoError(t, err)
	assert.Equal(t, "2001:db8::1", entry.Value)
}

func TestNormalizeRejectsIPv6Zone(t *testing.T) {
	_, err := normalize("fe80::1%en0", "ip6")
	assert.Error(t, err)
}

func TestNormalizeRejectsIPv4MappedIPv6(t *testing.T) {
	_, err := normalize("::ffff:1.2.3.4", "ip6")
	assert.Error(t, err)
}

func TestNormalizeRejectsIPv6InIPv4Field(t *testing.T) {
	_, err := normalize("::1", "ip4")
	assert.Error(t, err)
}

func TestNormalizeMasksIPv4CIDR(t *testing.T) {
	entry, err := normalize("192.0.2.123/24", "cidr4")
	require.NoError(t, err)
	assert.Equal(t, "192.0.2.0/24", entry.Value)
}

func TestNormalizeMasksIPv6CIDR(t *testing.T) {
	entry, err := normalize("2001:db8::1/32", "cidr6")
	require.NoError(t, err)
	assert.Equal(t, "2001:db8::/32", entry.Value)
}

func TestNormalizeRejectsIPv4CIDRInIPv6Field(t *testing.T) {
	_, err := normalize("1.2.3.0/24", "cidr6")
	assert.Error(t, err)
}

func TestNormalizeRejectsInvalidPrefix(t *testing.T) {
	_, err := normalize("1.2.3.0/99", "cidr4")
	assert.Error(t, err)
}

func TestFetchAcceptsEmptyArrayResponse(t *testing.T) {
	p := responseProvider(t, `[]`)
	result, err := p.Fetch(context.Background(), "unknown", resolver.Query{Kind: resolver.IPs, Family: resolver.IPv4})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Succeeded)
	assert.Empty(t, result.Entries)
}

func TestFetchAcceptsEmptyObjectResponse(t *testing.T) {
	p := responseProvider(t, `{}`)
	result, err := p.Fetch(context.Background(), "unknown", resolver.Query{Kind: resolver.IPs, Family: resolver.IPv4})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Succeeded)
	assert.Empty(t, result.Entries)
}

func TestFetchAcceptsNullPortalResponse(t *testing.T) {
	p := responseProvider(t, `{"site":null}`)
	result, err := p.Fetch(context.Background(), "unknown", resolver.Query{Kind: resolver.IPs, Family: resolver.IPv4})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Succeeded)
	assert.Empty(t, result.Entries)
}

func TestFetchRejectsNullResponse(t *testing.T) {
	p := responseProvider(t, `null`)
	result, err := p.Fetch(context.Background(), "unknown", resolver.Query{Kind: resolver.IPs, Family: resolver.IPv4})
	require.NoError(t, err)
	assert.Zero(t, result.Succeeded)
	assert.Len(t, result.Warnings, 1)
}

func TestFetchRejectsHTMLResponse(t *testing.T) {
	p := responseProvider(t, `<html>bad gateway</html>`)
	result, err := p.Fetch(context.Background(), "unknown", resolver.Query{Kind: resolver.IPs, Family: resolver.IPv4})
	require.NoError(t, err)
	assert.Zero(t, result.Succeeded)
	assert.Len(t, result.Warnings, 1)
}

func TestFetchRejectsErrorObjectResponse(t *testing.T) {
	p := responseProvider(t, `{"error":"oops"}`)
	result, err := p.Fetch(context.Background(), "unknown", resolver.Query{Kind: resolver.IPs, Family: resolver.IPv4})
	require.NoError(t, err)
	assert.Zero(t, result.Succeeded)
	assert.Len(t, result.Warnings, 1)
}

func TestFetchRejectsTrailingJSONData(t *testing.T) {
	p := responseProvider(t, `{"site":["1.2.3.4"]} trailing`)
	result, err := p.Fetch(context.Background(), "unknown", resolver.Query{Kind: resolver.IPs, Family: resolver.IPv4})
	require.NoError(t, err)
	assert.Zero(t, result.Succeeded)
	assert.Len(t, result.Warnings, 1)
}

func TestFetchReportsDeadline(t *testing.T) {
	p := newTestProvider(t, func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	t.Cleanup(cancel)
	result, err := p.Fetch(ctx, "youtube.com", resolver.Query{Kind: resolver.IPs, Family: resolver.IPv4})
	require.NoError(t, err)
	assert.Zero(t, result.Succeeded)
	require.Len(t, result.Warnings, 1)
	assert.Contains(t, result.Warnings[0], "deadline exceeded")
}

func TestFetchSelectsPortalForDomains(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "youtube.com", r.URL.Query().Get("site"))
		assert.Empty(t, r.URL.Query().Get("group"))
		writeResponse(t, w, `{"youtube.com":["youtube.com"]}`)
	})
	result, err := p.Fetch(context.Background(), "youtube.com", resolver.Query{Kind: resolver.Domains})
	require.NoError(t, err)
	assert.Equal(t, []resolver.Entry{{Value: "youtube.com", RuleType: "suffix"}}, result.Entries)
}
