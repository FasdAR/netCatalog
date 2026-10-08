package cmd

import (
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

// These cases catch filtering after discovery, fetching excluded sources, and
// accidentally retaining bindings from the combined catalogue.
func TestRunSourceFiltersCommands(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source string
		args   []string
		want   string
	}{
		{"get iplist", "iplist", []string{"get", "discord", "--type", "domains"}, "iplist.example\n"},
		{"get v2fly", "v2fly", []string{"get", "discord", "--type", "domains"}, "cdn.discord.com\nshared.com\n"},
		{"list iplist", "iplist", []string{"services", "list"}, "discord.com\n"},
		{"list v2fly", "v2fly", []string{"services", "list"}, "discord\nstandalone\n"},
		{"search iplist", "iplist", []string{"services", "search", "discord"}, "discord.com\n"},
		{"search v2fly", "v2fly", []string{"services", "search", "discord"}, "discord\n"},
		{"site iplist", "iplist", []string{"get", "discord.com", "--site", "--type", "domains"}, "iplist.example\n"},
		{"ips iplist", "iplist", []string{"get", "discord", "--type", "ips"}, "192.0.2.1\n"},
		{"cidrs iplist", "iplist", []string{"get", "discord", "--type", "cidrs"}, "192.0.2.0/24\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var iplistCalls, v2flyCalls atomic.Int32
			iplistServer := testServer(t, func(w http.ResponseWriter, r *http.Request) {
				iplistCalls.Add(1)
				switch r.URL.Query().Get("data") {
				case "domains":
					writeResponse(t, w, `{"discord.com":["iplist.example"]}`)
				case "ip4":
					writeResponse(t, w, `{"discord.com":["192.0.2.1"]}`)
				case "cidr4":
					writeResponse(t, w, `{"discord.com":["192.0.2.0/24"]}`)
				default:
					writeResponse(t, w, `{"discord.com":{}}`)
				}
			})
			v2flyServer := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
				v2flyCalls.Add(1)
				writeResponse(t, w, release)
			})
			args := append(tc.args, "--source", tc.source, "--iplist-url", iplistServer.URL, "--v2fly-url", v2flyServer.URL)
			assert.Equal(t, commandResult{out: tc.want}, execute(t, args...))
			if tc.source == "iplist" {
				assert.Positive(t, iplistCalls.Load())
				assert.Zero(t, v2flyCalls.Load())
			} else {
				assert.Zero(t, iplistCalls.Load())
				assert.Equal(t, int32(1), v2flyCalls.Load())
			}
		})
	}
}

func TestRunSourceIgnoresExcludedURL(t *testing.T) {
	for _, source := range []string{"iplist", "v2fly"} {
		t.Run(source, func(t *testing.T) {
			server := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
				if source == "iplist" {
					writeResponse(t, w, `{"discord.com":{}}`)
				} else {
					writeResponse(t, w, release)
				}
			})
			args := []string{"services", "search", "discord", "--source", source}
			want := "discord.com\n"
			if source == "iplist" {
				args = append(args, "--iplist-url", server.URL, "--v2fly-url", ":invalid")
			} else {
				args = append(args, "--v2fly-url", server.URL, "--iplist-url", ":invalid")
				want = "discord\n"
			}
			assert.Equal(t, commandResult{out: want}, execute(t, args...))
		})
	}
}

func TestRunSourceRejectsInvalidCombinationsBeforeHTTP(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"unknown get", []string{"get", "discord", "--type", "domains", "--source", "unknown"}, "iplist or v2fly"},
		{"unknown list", []string{"services", "list", "--source", "unknown"}, "iplist or v2fly"},
		{"unknown search", []string{"services", "search", "discord", "--source", "unknown"}, "iplist or v2fly"},
		{"ips", []string{"get", "discord", "--type", "ips", "--source", "v2fly"}, "domains"},
		{"cidrs", []string{"get", "discord", "--type", "cidrs", "--source", "v2fly"}, "domains"},
		{"site", []string{"get", "discord.com", "--type", "domains", "--site", "--source", "v2fly"}, "--site"},
		{"disabled get", []string{"get", "discord", "--type", "domains", "--source", "v2fly", "--v2fly-url", ""}, "--v2fly-url"},
		{"disabled list", []string{"services", "list", "--source", "v2fly", "--v2fly-url", ""}, "--v2fly-url"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				http.Error(w, "must not be called", http.StatusServiceUnavailable)
			})
			args := append([]string{"--iplist-url", server.URL, "--v2fly-url", server.URL}, tc.args...)
			result := execute(t, args...)
			assert.Equal(t, 2, result.code)
			assert.Empty(t, result.out)
			assert.Contains(t, result.err, tc.want)
			assert.Zero(t, calls.Load())
		})
	}
}

func TestRunDoesNotRetainSourceFlag(t *testing.T) {
	var downloads atomic.Int32
	iplistServer, v2flyServer := combinedSources(t, false, false, &downloads)
	args := []string{"get", "discord", "--type", "domains", "--iplist-url", iplistServer.URL, "--v2fly-url", v2flyServer.URL}
	assert.Equal(t, commandResult{out: "shared.com\n"}, execute(t, append(args, "--source", "iplist")...))
	assert.Equal(t, commandResult{out: "cdn.discord.com\nshared.com\n"}, execute(t, args...))
	assert.Equal(t, int32(1), downloads.Load())
}

func TestRunSourcesListRemainsCompleteWithSourceFlag(t *testing.T) {
	assert.Equal(t, commandResult{out: "iplist\tdomains,ips,cidrs\nv2fly\tdomains\n"}, execute(t, "sources", "list", "--source", "iplist"))
}
