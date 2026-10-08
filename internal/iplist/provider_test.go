package iplist

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"netcraze/internal/resolver"
)

func TestFetchPartialAndNoCache(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("group") != "" || r.URL.Query().Get("format") != "json" || r.URL.Query().Get("site") != "youtube.com" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		if r.URL.Query().Get("data") == "ip6" {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, `{"youtube.com":["1.2.3.4","invalid","::1","1.2.3.4"],"other.com":["5.6.7.8"]}`)
	}))
	defer server.Close()
	p, err := New(server.URL+"?site=stale&data=domains", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	engine := resolver.Resolver{Providers: []resolver.Provider{p}}
	service := resolver.Service{Domain: "youtube.com", Bindings: []resolver.Binding{{Source: p.ID(), Key: "youtube.com"}}}
	for i := 0; i < 2; i++ {
		result := engine.Fetch(context.Background(), resolver.Selection{Name: service.Domain, Services: []resolver.Service{service}}, resolver.Query{Kind: resolver.IPs, Family: resolver.All})
		if result.Succeeded != 1 || len(result.Entries) != 2 || len(result.Warnings) != 2 {
			t.Fatalf("unexpected result: %+v", result)
		}
		if result.Entries[0].Value != "1.2.3.4" || !reflect.DeepEqual(result.Entries[0].Sources, []string{"iplist"}) {
			t.Fatalf("unexpected entries: %+v", result.Entries)
		}
	}
	if calls != 4 {
		t.Fatalf("expected 4 live requests, got %d", calls)
	}
}

func TestListServices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("data") != "" || r.URL.Query().Get("group") != "" {
			t.Errorf("catalogue must not be filtered: %s", r.URL.RawQuery)
		}
		fmt.Fprint(w, `{"b.com":{"group":"youtube"},"a.com":{"group":"youtube"},"telegram.org":{"group":"messengers"},"c.com":{}}`)
	}))
	defer server.Close()
	p, _ := New(server.URL, server.Client())
	services, err := p.ListServices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 4 || services[0].Domain != "a.com" || services[3].Domain != "telegram.org" || !reflect.DeepEqual(services[3].Bindings, []resolver.Binding{{Source: "iplist", Key: "telegram.org"}}) {
		t.Fatalf("unexpected catalogue: %+v", services)
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		value, field, want string
		valid              bool
	}{
		{"*.EXAMPLE.COM.", "domains", "example.com", true},
		{"bad domain.com", "domains", "", false},
		{"-bad.com", "domains", "", false},
		{"1.2.3.4", "domains", "", false},
		{"regexp:test", "domains", "", false},
		{"xn--e1afmkfd.xn--p1ai", "domains", "xn--e1afmkfd.xn--p1ai", true},
		{"1.2.3.4", "ip4", "1.2.3.4", true},
		{"2001:0db8::1", "ip6", "2001:db8::1", true},
		{"fe80::1%en0", "ip6", "", false},
		{"::ffff:1.2.3.4", "ip6", "", false},
		{"::1", "ip4", "", false},
		{"192.0.2.123/24", "cidr4", "192.0.2.0/24", true},
		{"2001:db8::1/32", "cidr6", "2001:db8::/32", true},
		{"1.2.3.0/24", "cidr6", "", false},
		{"1.2.3.0/99", "cidr4", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.field+"/"+tt.value, func(t *testing.T) {
			entry, err := normalize(tt.value, tt.field)
			if (err == nil) != tt.valid || (tt.valid && entry.Value != tt.want) {
				t.Fatalf("got %+v, %v", entry, err)
			}
		})
	}
}

func TestInvalidAndEmptyResponses(t *testing.T) {
	for _, body := range []string{`[]`, `{}`, `null`, `<html>bad gateway</html>`, `{"error":"oops"}`, `{"site":null}`, `{"site":["1.2.3.4"]} trailing`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer server.Close()
			p, _ := New(server.URL, server.Client())
			result, err := p.Fetch(context.Background(), "unknown", resolver.Query{Kind: resolver.IPs, Family: resolver.IPv4})
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if body == `[]` || body == `{}` || body == `{"site":null}` {
				want = 1
			}
			if result.Succeeded != want {
				t.Fatalf("got %+v", result)
			}
		})
	}
}

func TestCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	p, _ := New(server.URL, server.Client())
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	result, _ := p.Fetch(ctx, "youtube.com", resolver.Query{Kind: resolver.IPs, Family: resolver.IPv4})
	if result.Succeeded != 0 || len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "deadline exceeded") {
		t.Fatalf("unexpected cancellation: %+v", result)
	}
}

func TestSiteSelection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("site") != "youtube.com" || r.URL.Query().Get("group") != "" {
			t.Errorf("bad query: %s", r.URL.RawQuery)
		}
		fmt.Fprint(w, `{"youtube.com":["youtube.com"]}`)
	}))
	defer server.Close()
	p, _ := New(server.URL, server.Client())
	result, _ := p.Fetch(context.Background(), "youtube.com", resolver.Query{Kind: resolver.Domains})
	if len(result.Entries) != 1 || result.Entries[0].RuleType != "suffix" {
		t.Fatalf("got %+v", result)
	}
}

func TestCatalogueKeepsValidPortals(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"TELEGRAM.ORG.":{"group":"messengers"},"bad domain":{},"error":"unavailable"}`)
	}))
	defer server.Close()
	p, _ := New(server.URL, server.Client())
	services, err := p.ListServices(context.Background())
	if err == nil || len(services) != 1 || services[0].Domain != "telegram.org" || services[0].Bindings[0].Key != "TELEGRAM.ORG." {
		t.Fatalf("unexpected catalogue: %+v, %v", services, err)
	}
}
