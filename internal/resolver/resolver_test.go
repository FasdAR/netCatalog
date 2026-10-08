package resolver

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeProvider struct {
	id           string
	result       Result
	err          error
	services     []Service
	catalogueErr error
	fetchKey     *string
}

func (p fakeProvider) ID() string             { return p.id }
func (fakeProvider) Capabilities() []DataKind { return []DataKind{Domains} }
func (p fakeProvider) ListServices(context.Context) ([]Service, error) {
	return p.services, p.catalogueErr
}
func (p fakeProvider) Fetch(_ context.Context, key string, _ Query) (Result, error) {
	if p.fetchKey != nil {
		*p.fetchKey = key
	}
	return p.result, p.err
}

func TestFetchPreservesDistinctRuleTypes(t *testing.T) {
	r := Resolver{Providers: []Provider{
		fakeProvider{id: "a", result: Result{Succeeded: 1, Entries: []Entry{{Value: "example.com", RuleType: "suffix"}, {Value: "example.com", RuleType: "suffix"}}}},
		fakeProvider{id: "b", result: Result{Succeeded: 1, Entries: []Entry{{Value: "example.com", RuleType: "suffix"}, {Value: "example.com", RuleType: "full"}}}},
	}}
	selection := Selection{Name: "example", Services: []Service{{Domain: "example.com", Bindings: []Binding{{Source: "a", Key: "list-a"}, {Source: "b", Key: "list-b"}}}}}
	result := r.Fetch(context.Background(), selection, Query{Kind: Domains})
	assert.Equal(t, 2, result.Succeeded)
	assert.Empty(t, result.Family)
	assert.Equal(t, []Entry{
		{Value: "example.com", RuleType: "full", Sources: []string{"b"}},
		{Value: "example.com", RuleType: "suffix", Sources: []string{"a", "b"}},
	}, result.Entries)
}

func TestFetchRetainsResultsWhenProviderFails(t *testing.T) {
	r := Resolver{Providers: []Provider{
		fakeProvider{id: "a", result: Result{Succeeded: 1, Entries: []Entry{{Value: "example.com"}}}},
		fakeProvider{id: "broken", err: errors.New("unavailable")},
	}}
	selection := Selection{Name: "example", Services: []Service{{Bindings: []Binding{{Source: "a", Key: "list"}, {Source: "broken", Key: "list"}}}}}
	result := r.Fetch(context.Background(), selection, Query{Kind: Domains})
	assert.Equal(t, 1, result.Succeeded)
	assert.Equal(t, []Entry{{Value: "example.com", Sources: []string{"a"}}}, result.Entries)
	assert.Equal(t, []string{"broken list: unavailable"}, result.Warnings)
}

func TestListServicesMergesNormalizedDomainBindings(t *testing.T) {
	r := Resolver{Providers: []Provider{
		fakeProvider{id: "a", services: []Service{
			{Domain: "TELEGRAM.ORG.", Bindings: []Binding{{Source: "a", Key: "entry-42"}, {Source: "a", Key: "entry-42"}}},
			{Domain: "other.org", Bindings: []Binding{{Source: "a", Key: "other"}}},
		}},
		fakeProvider{id: "b", services: []Service{{Domain: "telegram.org", Bindings: []Binding{{Source: "b", Key: "data/telegram"}}}}},
	}}
	result := r.ListServices(context.Background())
	assert.Equal(t, 2, result.Succeeded)
	assert.Equal(t, []Service{
		{Domain: "other.org", Bindings: []Binding{{Source: "a", Key: "other"}}},
		{Domain: "telegram.org", Bindings: []Binding{{Source: "a", Key: "entry-42"}, {Source: "b", Key: "data/telegram"}}},
	}, result.Services)
}

func TestListServicesReportsFailedProvider(t *testing.T) {
	r := Resolver{Providers: []Provider{
		fakeProvider{id: "empty"},
		fakeProvider{id: "broken", catalogueErr: errors.New("offline")},
	}}
	result := r.ListServices(context.Background())
	assert.Equal(t, 1, result.Succeeded)
	assert.Equal(t, []string{"broken catalogue: offline"}, result.Warnings)
}

func TestFetchDispatchesOpaqueKeys(t *testing.T) {
	var keyA, keyB, unrelated string
	r := Resolver{Providers: []Provider{
		fakeProvider{id: "a", fetchKey: &keyA},
		fakeProvider{id: "b", fetchKey: &keyB},
		fakeProvider{id: "unrelated", fetchKey: &unrelated},
	}}
	selection := Selection{Services: []Service{{Bindings: []Binding{{Source: "a", Key: "entry-42"}, {Source: "b", Key: "data/telegram"}}}}}
	r.Fetch(context.Background(), selection, Query{Kind: Domains})
	assert.Equal(t, "entry-42", keyA)
	assert.Equal(t, "data/telegram", keyB)
	assert.Empty(t, unrelated)
}

func searchCatalogue() Catalogue {
	return Catalogue{Services: []Service{{Domain: "telegram.org"}, {Domain: "telegram.org.example.com"}, {Domain: "youtube.com"}}}
}

func TestSearchReturnsSubstringMatches(t *testing.T) {
	result := searchCatalogue().Search("TELEGRAM")
	assert.Equal(t, []Service{{Domain: "telegram.org"}, {Domain: "telegram.org.example.com"}}, result)
}

func TestSearchDoesNotExposeBindingKeys(t *testing.T) {
	c := Catalogue{Services: []Service{{Domain: "telegram.org", Bindings: []Binding{{Source: "a", Key: "messenger-42"}}}}}
	assert.Empty(t, c.Search("messenger"))
}

func TestResolveSelectsExactDomain(t *testing.T) {
	selection, err := searchCatalogue().Resolve("telegram.org")
	require.NoError(t, err)
	assert.Equal(t, Selection{Name: "telegram.org", Services: []Service{{Domain: "telegram.org"}}}, selection)
}

func TestResolveNormalizesInput(t *testing.T) {
	selection, err := searchCatalogue().Resolve(" TELEGRAM.ORG. ")
	require.NoError(t, err)
	assert.Equal(t, Selection{Name: "telegram.org", Services: []Service{{Domain: "telegram.org"}}}, selection)
}

func TestResolveSelectsShortName(t *testing.T) {
	selection, err := searchCatalogue().Resolve("youtube")
	require.NoError(t, err)
	assert.Equal(t, Selection{Name: "youtube.com", Services: []Service{{Domain: "youtube.com"}}}, selection)
}

func TestResolveSelectsUniqueSubstring(t *testing.T) {
	selection, err := searchCatalogue().Resolve("yout")
	require.NoError(t, err)
	assert.Equal(t, "youtube.com", selection.Name)
}

func TestResolveRejectsAmbiguousSubstring(t *testing.T) {
	_, err := searchCatalogue().Resolve("tele")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ambiguous")
}

func TestResolveRejectsMissingService(t *testing.T) {
	_, err := searchCatalogue().Resolve("missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no portal matching")
}

func TestResolveRejectsEmptyTerm(t *testing.T) {
	_, err := searchCatalogue().Resolve(" ")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not be empty")
}

func TestListServicesRetainsPartialProviderRecords(t *testing.T) {
	r := Resolver{Providers: []Provider{fakeProvider{id: "a", catalogueErr: errors.New("skipped bad entry"), services: []Service{
		{Domain: "telegram.org", Bindings: []Binding{{Source: "a", Key: "telegram"}}},
	}}}}
	result := r.ListServices(context.Background())
	assert.Equal(t, 1, result.Succeeded)
	assert.Len(t, result.Warnings, 1)
	assert.Equal(t, []Service{{Domain: "telegram.org", Bindings: []Binding{{Source: "a", Key: "telegram"}}}}, result.Services)
}

func TestListServicesDiscardsForeignBinding(t *testing.T) {
	r := Resolver{Providers: []Provider{fakeProvider{id: "a", services: []Service{
		{Domain: "invalid.org", Bindings: []Binding{{Source: "other", Key: "wrong"}}},
	}}}}
	assert.Empty(t, r.ListServices(context.Background()).Services)
}

func discordCatalogue() Catalogue {
	return Catalogue{Services: []Service{
		{Domain: "discord.com"}, {Domain: "discord.gg"}, {Domain: "discord.media"},
		{Domain: "discord-tools.com"}, {Domain: "notdiscord.com"}, {Domain: "cdn.discord.com"},
	}}
}

func TestResolveCombinesMatchingFirstLabels(t *testing.T) {
	selection, err := discordCatalogue().Resolve("discord")
	require.NoError(t, err)
	assert.Equal(t, Selection{Name: "discord", Services: []Service{{Domain: "discord.com"}, {Domain: "discord.gg"}, {Domain: "discord.media"}}}, selection)
}

func TestResolveNormalizesShortName(t *testing.T) {
	selection, err := discordCatalogue().Resolve(" DISCORD ")
	require.NoError(t, err)
	assert.Equal(t, Selection{Name: "discord", Services: []Service{{Domain: "discord.com"}, {Domain: "discord.gg"}, {Domain: "discord.media"}}}, selection)
}

func TestResolveDoesNotBroadenExactDomain(t *testing.T) {
	selection, err := discordCatalogue().Resolve("DISCORD.COM.")
	require.NoError(t, err)
	assert.Equal(t, Selection{Name: "discord.com", Services: []Service{{Domain: "discord.com"}}}, selection)
}

func TestFetchDeduplicatesBindingsAcrossPortals(t *testing.T) {
	r := Resolver{Providers: []Provider{fakeProvider{id: "a", result: Result{Succeeded: 1, Entries: []Entry{{Value: "shared.com", RuleType: "suffix"}}}}}}
	selection := Selection{Name: "discord", Services: []Service{
		{Domain: "discord.com", Bindings: []Binding{{Source: "a", Key: "shared-list"}}},
		{Domain: "discord.gg", Bindings: []Binding{{Source: "a", Key: "shared-list"}}},
	}}
	result := r.Fetch(context.Background(), selection, Query{Kind: Domains})
	assert.Equal(t, 1, result.Succeeded)
	assert.Equal(t, []Entry{{Value: "shared.com", RuleType: "suffix", Sources: []string{"a"}}}, result.Entries)
	assert.Equal(t, []string{"discord.com", "discord.gg"}, result.Portals)
}

func namedCatalogue() Catalogue {
	return Catalogue{Services: []Service{
		{Name: "discord", Bindings: []Binding{{Source: "v2fly", Key: "discord"}}},
		{Domain: "discord.com", Bindings: []Binding{{Source: "iplist", Key: "discord.com"}}},
		{Domain: "discord.gg", Bindings: []Binding{{Source: "iplist", Key: "discord.gg"}}},
		{Name: "standalone", Bindings: []Binding{{Source: "v2fly", Key: "standalone"}}},
	}}
}

func TestResolveCombinesNamedListWithPortals(t *testing.T) {
	c := namedCatalogue()
	selection, err := c.Resolve("discord")
	require.NoError(t, err)
	assert.Equal(t, Selection{Name: "discord", Services: c.Services[:3]}, selection)
}

func TestResolveExactDomainExcludesNamedList(t *testing.T) {
	selection, err := namedCatalogue().Resolve("discord.com")
	require.NoError(t, err)
	assert.Equal(t, Selection{Name: "discord.com", Services: []Service{{Domain: "discord.com", Bindings: []Binding{{Source: "iplist", Key: "discord.com"}}}}}, selection)
}

func TestResolveSelectsStandaloneList(t *testing.T) {
	selection, err := namedCatalogue().Resolve("standalone")
	require.NoError(t, err)
	assert.Equal(t, Selection{Name: "standalone", Services: []Service{{Name: "standalone", Bindings: []Binding{{Source: "v2fly", Key: "standalone"}}}}}, selection)
}
