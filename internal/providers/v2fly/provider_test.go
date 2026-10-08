package v2fly

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"netCatalog/internal/resolver"
)

const fixture = `lists:
  - name: discord
    length: 6
    rules:
      - "domain:discord.com"
      - "domain:DISCORD.GG:@ads:@cn"
      - "full:exact.discord.com"
      - "keyword:discord"
      - 'regexp:^discord.*'
      - "domain:bad domain"
  - name: categorymessengers
    length: 1
    rules: ["domain:excluded.com"]
  - name: discord-extra
    length: 1
    rules: ["domain:excluded.net"]
  - name: telegram
    length: 1
    rules: ["domain:telegram.org"]
`

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

func TestListServicesDiscoversNamedLists(t *testing.T) {
	p := responseProvider(t, fixture)
	services, err := p.ListServices(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []resolver.Service{
		{Name: "discord", Bindings: []resolver.Binding{{Source: "v2fly", Key: "discord"}}},
		{Name: "telegram", Bindings: []resolver.Binding{{Source: "v2fly", Key: "telegram"}}},
	}, services)
}

func TestFetchReturnsOnlyDomainRules(t *testing.T) {
	p := responseProvider(t, fixture)
	result, err := p.Fetch(context.Background(), "discord", resolver.Query{Kind: resolver.Domains})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Succeeded)
	assert.Len(t, result.Warnings, 2)
	assert.Equal(t, []resolver.Entry{{Value: "discord.com", RuleType: "suffix"}, {Value: "discord.gg", RuleType: "suffix"}}, result.Entries)
}

func TestFetchReusesReleaseWithinProvider(t *testing.T) {
	var downloads atomic.Int32
	p := newTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		downloads.Add(1)
		writeResponse(t, w, fixture)
	})
	_, err := p.Fetch(context.Background(), "discord", resolver.Query{Kind: resolver.Domains})
	require.NoError(t, err)
	_, err = p.Fetch(context.Background(), "telegram", resolver.Query{Kind: resolver.Domains})
	require.NoError(t, err)
	assert.Equal(t, int32(1), downloads.Load())
}

func TestListServicesDownloadsForEachProviderInstance(t *testing.T) {
	var downloads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		downloads.Add(1)
		writeResponse(t, w, fixture)
	}))
	t.Cleanup(server.Close)
	first, err := New(server.URL, server.Client())
	require.NoError(t, err)
	second, err := New(server.URL, server.Client())
	require.NoError(t, err)
	_, err = first.ListServices(context.Background())
	require.NoError(t, err)
	_, err = second.ListServices(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int32(2), downloads.Load())
}

func TestFetchRejectsExcludedList(t *testing.T) {
	p := responseProvider(t, fixture)
	_, err := p.Fetch(context.Background(), "discord-extra", resolver.Query{Kind: resolver.Domains})
	assert.Error(t, err)
}

func TestFetchRejectsIPRequests(t *testing.T) {
	p := responseProvider(t, fixture)
	_, err := p.Fetch(context.Background(), "discord", resolver.Query{Kind: resolver.IPs})
	assert.Error(t, err)
}

func TestListServicesRejectsHTML(t *testing.T) {
	p := responseProvider(t, "<html>bad gateway</html>")
	_, err := p.ListServices(context.Background())
	assert.Error(t, err)
}

func TestListServicesRejectsMissingLists(t *testing.T) {
	p := responseProvider(t, "{}")
	_, err := p.ListServices(context.Background())
	assert.Error(t, err)
}

func TestListServicesRejectsNullLists(t *testing.T) {
	p := responseProvider(t, "lists: null")
	_, err := p.ListServices(context.Background())
	assert.Error(t, err)
}

func TestListServicesRejectsMultipleDocuments(t *testing.T) {
	p := responseProvider(t, "lists: []\n---\ninvalid: true")
	_, err := p.ListServices(context.Background())
	assert.Error(t, err)
}

func TestListServicesRejectsInconsistentLength(t *testing.T) {
	p := responseProvider(t, `lists: [{name: discord, length: 5, rules: ["domain:discord.com"]}]`)
	_, err := p.ListServices(context.Background())
	assert.Error(t, err)
}

func TestListServicesRejectsDuplicateNames(t *testing.T) {
	p := responseProvider(t, `lists: [{name: discord, length: 0, rules: []}, {name: discord, length: 0, rules: []}]`)
	_, err := p.ListServices(context.Background())
	assert.Error(t, err)
}

func TestListServicesReportsHTTPFailure(t *testing.T) {
	p := newTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	})
	_, err := p.ListServices(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "503")
}

func TestListServicesReportsCancellation(t *testing.T) {
	p := responseProvider(t, fixture)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := p.ListServices(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}
