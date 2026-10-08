package resolver

import (
	"context"
	"errors"
	"reflect"
	"testing"
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

func TestMergePreservesRulesAndProvenance(t *testing.T) {
	r := Resolver{Providers: []Provider{
		fakeProvider{id: "a", result: Result{Succeeded: 1, Entries: []Entry{{Value: "example.com", RuleType: "suffix"}, {Value: "example.com", RuleType: "suffix"}}}},
		fakeProvider{id: "b", result: Result{Succeeded: 1, Entries: []Entry{{Value: "example.com", RuleType: "suffix"}, {Value: "example.com", RuleType: "full"}}}},
		fakeProvider{id: "broken", err: errors.New("unavailable")},
	}}
	service := Service{Domain: "example.com", Bindings: []Binding{{Source: "a", Key: "list-a"}, {Source: "b", Key: "category-b"}, {Source: "broken", Key: "example"}}}
	result := r.Fetch(context.Background(), Selection{Name: service.Domain, Services: []Service{service}}, Query{Kind: Domains})
	if len(result.Entries) != 2 || result.Succeeded != 2 || len(result.Warnings) != 1 || result.Family != "" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Entries[0].RuleType != "full" || !reflect.DeepEqual(result.Entries[1].Sources, []string{"a", "b"}) {
		t.Fatalf("unexpected entries: %+v", result.Entries)
	}
}

func TestDynamicCatalogueAndOpaqueBindings(t *testing.T) {
	var keyA, keyB, unrelated string
	r := Resolver{Providers: []Provider{
		fakeProvider{id: "a", fetchKey: &keyA, services: []Service{
			{Domain: "TELEGRAM.ORG.", Bindings: []Binding{{Source: "a", Key: "messenger-entry-42"}, {Source: "a", Key: "messenger-entry-42"}}},
			{Domain: "other.org", Bindings: []Binding{{Source: "a", Key: "other"}}},
		}},
		fakeProvider{id: "b", fetchKey: &keyB, services: []Service{{Domain: "telegram.org", Bindings: []Binding{{Source: "b", Key: "data/telegram"}}}}},
		fakeProvider{id: "broken", catalogueErr: errors.New("offline")},
		fakeProvider{id: "unrelated", fetchKey: &unrelated},
	}}
	catalogue := r.ListServices(context.Background())
	if catalogue.Succeeded != 3 || len(catalogue.Warnings) != 1 || len(catalogue.Services) != 2 {
		t.Fatalf("unexpected catalogue: %+v", catalogue)
	}
	matches := catalogue.Search("TELEGRAM")
	if len(matches) != 1 || matches[0].Domain != "telegram.org" || len(matches[0].Bindings) != 2 {
		t.Fatalf("bad matches: %+v", matches)
	}
	if len(catalogue.Search("messenger")) != 0 {
		t.Fatal("source key leaked into search")
	}
	service, err := catalogue.Resolve("telegram")
	if err != nil {
		t.Fatal(err)
	}
	r.Fetch(context.Background(), service, Query{Kind: Domains})
	if keyA != "messenger-entry-42" || keyB != "data/telegram" || unrelated != "" {
		t.Fatalf("wrong binding dispatch: %q %q %q", keyA, keyB, unrelated)
	}
}

func TestResolveExactUniqueAmbiguousAndMissing(t *testing.T) {
	catalogue := Catalogue{Services: []Service{{Domain: "telegram.org"}, {Domain: "telegram.org.example.com"}, {Domain: "youtube.com"}}}
	for _, term := range []string{"telegram.org", " TELEGRAM.ORG. ", "youtube"} {
		service, err := catalogue.Resolve(term)
		if err != nil || service.Name == "" {
			t.Fatalf("%q: %+v %v", term, service, err)
		}
	}
	for _, term := range []string{"tele", "missing", " "} {
		if _, err := catalogue.Resolve(term); err == nil {
			t.Fatalf("expected resolution error for %q", term)
		}
	}
	if len(catalogue.Search("telegram")) != 2 {
		t.Fatal("search should show all candidates")
	}
}

func TestCatalogueRetainsPartialResults(t *testing.T) {
	r := Resolver{Providers: []Provider{fakeProvider{id: "a", catalogueErr: errors.New("skipped bad entry"), services: []Service{
		{Domain: "telegram.org", Bindings: []Binding{{Source: "a", Key: "telegram"}}},
		{Domain: "invalid.org", Bindings: []Binding{{Source: "different-provider", Key: "wrong"}}},
	}}}}
	catalogue := r.ListServices(context.Background())
	if len(catalogue.Services) != 1 || len(catalogue.Warnings) != 1 || catalogue.Succeeded != 1 {
		t.Fatalf("got %+v", catalogue)
	}
}

func TestResolveCombinesMatchingLabels(t *testing.T) {
	catalogue := Catalogue{Services: []Service{
		{Domain: "discord.com"}, {Domain: "discord.gg"}, {Domain: "discord.media"},
		{Domain: "discord-tools.com"}, {Domain: "notdiscord.com"}, {Domain: "cdn.discord.com"},
	}}
	for _, term := range []string{"discord", " DISCORD "} {
		selection, err := catalogue.Resolve(term)
		if err != nil {
			t.Fatal(err)
		}
		domains := []string{}
		for _, service := range selection.Services {
			domains = append(domains, service.Domain)
		}
		if selection.Name != "discord" || !reflect.DeepEqual(domains, []string{"discord.com", "discord.gg", "discord.media"}) {
			t.Fatalf("unexpected selection: %+v", selection)
		}
	}
	selection, err := catalogue.Resolve("DISCORD.COM.")
	if err != nil || selection.Name != "discord.com" || len(selection.Services) != 1 {
		t.Fatalf("exact match broadened: %+v %v", selection, err)
	}
}

func TestFetchDeduplicatesBindingsAcrossPortals(t *testing.T) {
	r := Resolver{Providers: []Provider{fakeProvider{id: "a", result: Result{Succeeded: 1, Entries: []Entry{{Value: "shared.com", RuleType: "suffix"}}}}}}
	selection := Selection{Name: "discord", Services: []Service{
		{Domain: "discord.com", Bindings: []Binding{{Source: "a", Key: "shared-list"}}},
		{Domain: "discord.gg", Bindings: []Binding{{Source: "a", Key: "shared-list"}}},
	}}
	result := r.Fetch(context.Background(), selection, Query{Kind: Domains})
	if result.Succeeded != 1 || len(result.Entries) != 1 || !reflect.DeepEqual(result.Portals, []string{"discord.com", "discord.gg"}) {
		t.Fatalf("unexpected result: %+v", result)
	}
}
