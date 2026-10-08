package cli

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestCLI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("group") != "" {
			t.Errorf("group leaked into request: %s", r.URL.RawQuery)
		}
		if r.URL.Query().Get("data") != "" && r.URL.Query().Get("site") != "youtube.com" {
			t.Errorf("wrong portal selection: %s", r.URL.RawQuery)
		}
		switch r.URL.Query().Get("data") {
		case "domains":
			fmt.Fprint(w, `{"youtube.com":["YouTube.com","*.youtube.com","bad domain"]}`)
		case "ip4":
			fmt.Fprint(w, `{"youtube.com":["1.2.3.4"]}`)
		case "ip6":
			http.Error(w, "oops", 503)
		default:
			fmt.Fprint(w, `{"youtube.com":{"group":"youtube"},"openai.com":{"group":"ai"}}`)
		}
	}))
	defer server.Close()
	tests := []struct {
		name     string
		args     []string
		code     int
		contains string
	}{
		{"domains", []string{"get", "youtube", "--type", "domains"}, 0, "youtube.com\n"},
		{"partial", []string{"get", "youtube", "--type", "ips", "--family", "all", "--info"}, 0, "1.2.3.4\n"},
		{"failed", []string{"get", "youtube", "--type", "ips", "--family", "ipv6"}, 1, ""},
		{"list", []string{"services", "list"}, 0, "youtube.com\n"},
		{"search", []string{"services", "search", "OPENAI"}, 0, "openai.com\n"},
		{"empty search", []string{"services", "search", "missing"}, 0, ""},
		{"exact", []string{"get", "youtube.com", "--type", "domains"}, 0, "youtube.com\n"},
		{"raw site", []string{"get", "youtube.com", "--site", "--type", "domains"}, 0, "youtube.com\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, diagnostics bytes.Buffer
			args := append(append([]string{}, tt.args...), "--url", server.URL)
			code := Run(context.Background(), args, &out, &diagnostics)
			if code != tt.code || !strings.Contains(out.String(), tt.contains) {
				t.Fatalf("code=%d out=%q err=%q", code, out.String(), diagnostics.String())
			}
			if tt.name == "partial" && !strings.Contains(diagnostics.String(), "503") {
				t.Fatal("missing partial failure warning")
			}
			if tt.name == "domains" || tt.name == "exact" || tt.name == "raw site" {
				if out.String() != "youtube.com\n" {
					t.Fatalf("expected one bare domain, got %q", out.String())
				}
			}
		})
	}
}

func TestTelegramSearchAndResolution(t *testing.T) {
	dataCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("data") == "" {
			fmt.Fprint(w, `{"telegram.org":{"group":"messengers"},"telegram.org.example.com":{"group":"messengers"},"whatsapp.com":{"group":"messengers"}}`)
			return
		}
		dataCalls++
		if r.URL.Query().Get("site") != "telegram.org" || r.URL.Query().Get("group") != "" {
			t.Errorf("wrong query: %s", r.URL.RawQuery)
		}
		fmt.Fprint(w, `{"telegram.org":["192.0.2.1"]}`)
	}))
	defer server.Close()
	for _, tt := range []struct {
		args            []string
		code            int
		out, diagnostic string
	}{
		{[]string{"services", "search", "telegram"}, 0, "telegram.org\ntelegram.org.example.com\n", ""},
		{[]string{"services", "search", "messengers"}, 0, "", ""},
		{[]string{"get", "tele", "--type", "ips"}, 2, "", "ambiguous"},
		{[]string{"get", "telegram.org", "--type", "ips"}, 0, "192.0.2.1\n", ""},
		{[]string{"get", "missing", "--type", "ips"}, 2, "", "no portal matching"},
	} {
		var out, diagnostics bytes.Buffer
		args := append(tt.args, "--url", server.URL)
		code := Run(context.Background(), args, &out, &diagnostics)
		if code != tt.code || out.String() != tt.out || !strings.Contains(diagnostics.String(), tt.diagnostic) {
			t.Fatalf("%v: code=%d out=%q err=%q", args, code, out.String(), diagnostics.String())
		}
	}
	if dataCalls != 1 {
		t.Fatalf("ambiguous/missing selections must not fetch data, got %d calls", dataCalls)
	}
}

func TestCatalogueFailureAndSiteBypass(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("data") == "" {
			http.Error(w, "catalogue unavailable", 503)
			return
		}
		fmt.Fprint(w, `{"telegram.org":["192.0.2.1"]}`)
	}))
	defer server.Close()
	for _, tt := range []struct {
		args []string
		code int
	}{
		{[]string{"services", "list"}, 1},
		{[]string{"get", "telegram", "--type", "ips"}, 1},
		{[]string{"get", "telegram.org", "--site", "--type", "ips"}, 0},
	} {
		var out, diagnostics bytes.Buffer
		if code := Run(context.Background(), append(tt.args, "--url", server.URL), &out, &diagnostics); code != tt.code {
			t.Fatalf("%v: code=%d diagnostics=%q", tt.args, code, diagnostics.String())
		}
	}
}

func TestInvalidArguments(t *testing.T) {
	tests := [][]string{
		{"get"}, {"get", "youtube"}, {"get", "youtube", "--type", "bad"},
		{"get", "youtube", "--type", "domains", "--family", "ipv4"},
		{"get", "youtube", "--type", "ips", "--family", "bad"},
		{"get", "youtube", "--type", "ips", "--timeout", "0"},
		{"get", "youtube", "--type", "ips", "--url", "://bad"},
		{"get", "youtube", "--type", "ips", "--format", "json"},
		{"get", "youtube", "--type", "domains", "--format", "text"},
		{"services", "list", "--format", "json"},
		{"services", "search", "youtube", "--format", "text"},
		{"get", " ", "--type", "ips"}, {"get", "youtube", "--type", "ips", "extra"},
		{"services", "search"}, {"services", "bad"}, {"unknown"},
	}
	for _, args := range tests {
		var out, diagnostics bytes.Buffer
		if code := Run(context.Background(), args, &out, &diagnostics); code != 2 {
			t.Fatalf("%v: code=%d", args, code)
		}
	}
}

func TestHelpAndSources(t *testing.T) {
	for _, args := range [][]string{nil, {"--help"}, {"sources", "list"}, {"get", "youtube", "--help"}} {
		var out, diagnostics bytes.Buffer
		if code := Run(context.Background(), args, &out, &diagnostics); code != 0 {
			t.Fatalf("%v: code=%d", args, code)
		}
		if strings.Contains(out.String()+diagnostics.String(), "--format") {
			t.Fatal("removed flag is still advertised")
		}
	}
}

func TestDiscordCombinesPortalsForEveryDataKind(t *testing.T) {
	for _, kind := range []string{"domains", "ips", "cidrs"} {
		t.Run(kind, func(t *testing.T) {
			calls := []string{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("data") == "" {
					fmt.Fprint(w, `{"discord.com":{},"discord.gg":{},"discord.media":{},"discord-tools.com":{},"notdiscord.com":{}}`)
					return
				}
				site := r.URL.Query().Get("site")
				calls = append(calls, site)
				if r.URL.Query().Get("group") != "" {
					t.Errorf("unexpected group request")
				}
				if site == "discord.media" {
					http.Error(w, "unavailable", 503)
					return
				}
				values := map[string]string{}
				switch r.URL.Query().Get("data") {
				case "domains":
					values = map[string]string{"discord.com": "cdn.discord.com", "discord.gg": "discord.gg"}
				case "ip4":
					values = map[string]string{"discord.com": "192.0.2.1", "discord.gg": "192.0.2.2"}
				case "cidr4":
					values = map[string]string{"discord.com": "192.0.2.0/24", "discord.gg": "198.51.100.0/24"}
				default:
					t.Errorf("unexpected field: %s", r.URL.Query().Get("data"))
				}
				fmt.Fprintf(w, `{%q:[%q,%q]}`, site, values[site], values[site])
			}))
			defer server.Close()
			var out, diagnostics bytes.Buffer
			code := Run(context.Background(), []string{"get", "discord", "--type", kind, "--info", "--url", server.URL}, &out, &diagnostics)
			want := map[string]string{
				"domains": "cdn.discord.com\ndiscord.gg\n",
				"ips":     "192.0.2.1\n192.0.2.2\n",
				"cidrs":   "192.0.2.0/24\n198.51.100.0/24\n",
			}[kind]
			if code != 0 || out.String() != want || !strings.Contains(diagnostics.String(), "503") {
				t.Fatalf("code=%d output=%q diagnostics=%s", code, out.String(), diagnostics.String())
			}
			if !reflect.DeepEqual(calls, []string{"discord.com", "discord.gg", "discord.media"}) {
				t.Fatalf("wrong requests: %v", calls)
			}
			calls = nil
			out.Reset()
			diagnostics.Reset()
			code = Run(context.Background(), []string{"get", "discord.com", "--type", kind, "--url", server.URL}, &out, &diagnostics)
			if code != 0 || !reflect.DeepEqual(calls, []string{"discord.com"}) {
				t.Fatalf("exact selection broadened: code=%d calls=%v", code, calls)
			}
		})
	}
}

func TestWarningsRequireInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("data") {
		case "":
			fmt.Fprint(w, `{"telegram.org":{},"invalid@key":{}}`)
		case "domains":
			fmt.Fprint(w, `{"telegram.org":["telegram.org","bad domain"]}`)
		case "ip4":
			fmt.Fprint(w, `{"telegram.org":["192.0.2.1"]}`)
		case "ip6":
			http.Error(w, "unavailable", 503)
		}
	}))
	defer server.Close()
	for _, command := range [][]string{
		{"get", "telegram", "--type", "domains"},
		{"get", "telegram", "--type", "ips", "--family", "all"},
		{"services", "list"},
		{"services", "search", "telegram"},
	} {
		var expected string
		for _, info := range []bool{false, true} {
			args := append(append([]string{}, command...), "--url", server.URL)
			if info {
				args = append(args, "--info")
			}
			var out, diagnostics bytes.Buffer
			if code := Run(context.Background(), args, &out, &diagnostics); code != 0 {
				t.Fatalf("%v: code=%d diagnostics=%q", args, code, diagnostics.String())
			}
			if !info {
				expected = out.String()
				if diagnostics.Len() != 0 {
					t.Fatalf("warnings visible by default: %q", diagnostics.String())
				}
			} else {
				if out.String() != expected || !strings.Contains(diagnostics.String(), "warning:") || strings.Contains(diagnostics.String(), "\x1b") {
					t.Fatalf("bad --info output: out=%q diagnostics=%q", out.String(), diagnostics.String())
				}
			}
		}
	}
}

func TestTotalFailureAlwaysReportsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "unavailable", 503) }))
	defer server.Close()
	for _, args := range [][]string{
		{"services", "list"},
		{"get", "telegram.org", "--site", "--type", "domains"},
	} {
		var out, diagnostics bytes.Buffer
		code := Run(context.Background(), append(args, "--url", server.URL), &out, &diagnostics)
		if code != 1 || out.Len() != 0 || !strings.Contains(diagnostics.String(), "error:") || strings.Contains(diagnostics.String(), "warning:") {
			t.Fatalf("code=%d out=%q diagnostics=%q", code, out.String(), diagnostics.String())
		}
	}
}
