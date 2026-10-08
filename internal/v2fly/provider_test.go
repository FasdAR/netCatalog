package v2fly

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"netcraze/internal/resolver"
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

func TestDiscoveryAndDomainOnlyRules(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; fmt.Fprint(w, fixture) }))
	defer server.Close()
	for invocation := 0; invocation < 2; invocation++ {
		p, err := New(server.URL, server.Client())
		if err != nil {
			t.Fatal(err)
		}
		services, err := p.ListServices(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(services) != 2 || services[0].Name != "discord" || services[0].Domain != "" || services[1].Name != "telegram" {
			t.Fatalf("bad catalogue: %+v", services)
		}
		result, err := p.Fetch(context.Background(), "discord", resolver.Query{Kind: resolver.Domains})
		if err != nil {
			t.Fatal(err)
		}
		if result.Succeeded != 1 || len(result.Warnings) != 2 || !reflect.DeepEqual(result.Entries, []resolver.Entry{{Value: "discord.com", RuleType: "suffix"}, {Value: "discord.gg", RuleType: "suffix"}}) {
			t.Fatalf("bad rules: %+v", result)
		}
		if _, err := p.Fetch(context.Background(), "telegram", resolver.Query{Kind: resolver.Domains}); err != nil {
			t.Fatal(err)
		}
		if _, err := p.Fetch(context.Background(), "discord-extra", resolver.Query{Kind: resolver.Domains}); err == nil {
			t.Fatal("excluded list is fetchable")
		}
		if _, err := p.Fetch(context.Background(), "discord", resolver.Query{Kind: resolver.IPs}); err == nil {
			t.Fatal("IP requests must be unsupported")
		}
	}
	if calls != 2 {
		t.Fatalf("expected one download per invocation, got %d", calls)
	}
}

func TestBadReleaseAndHTTPFailure(t *testing.T) {
	for _, body := range []string{"<html>bad gateway</html>", "{}", "lists: null", "lists: []\n---\ninvalid: true", `lists: [{name: discord, length: 5, rules: ["domain:discord.com"]}]`, `lists: [{name: discord, length: 0, rules: []}, {name: discord, length: 0, rules: []}]`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer server.Close()
			p, _ := New(server.URL, server.Client())
			if _, err := p.ListServices(context.Background()); err == nil {
				t.Fatal("expected schema error")
			}
		})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "unavailable", 503) }))
	defer server.Close()
	p, _ := New(server.URL, server.Client())
	if _, err := p.ListServices(context.Background()); err == nil {
		t.Fatal("expected HTTP failure")
	}
}

func TestCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, fixture) }))
	defer server.Close()
	p, _ := New(server.URL, server.Client())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.ListServices(ctx); err == nil {
		t.Fatal("expected cancellation")
	}
}
