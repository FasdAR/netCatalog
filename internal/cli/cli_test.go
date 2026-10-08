package cli

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

type commandResult struct {
	code int
	out  string
	err  string
}

func writeResponse(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	_, err := fmt.Fprint(w, body)
	assert.NoError(t, err)
}

func execute(t *testing.T, args ...string) commandResult {
	t.Helper()
	var out, diagnostics bytes.Buffer
	code := run(context.Background(), args, &out, &diagnostics, "")
	return commandResult{code: code, out: out.String(), err: diagnostics.String()}
}

func testServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func youtubeServer(t *testing.T) *httptest.Server {
	t.Helper()
	return testServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.URL.Query().Get("group"))
		if r.URL.Query().Get("data") != "" {
			assert.Equal(t, "youtube.com", r.URL.Query().Get("site"))
		}
		switch r.URL.Query().Get("data") {
		case "domains":
			writeResponse(t, w, `{"youtube.com":["YouTube.com","*.youtube.com","bad domain"]}`)
		case "ip4":
			writeResponse(t, w, `{"youtube.com":["1.2.3.4"]}`)
		case "ip6":
			http.Error(w, "oops", http.StatusServiceUnavailable)
		default:
			writeResponse(t, w, `{"youtube.com":{"group":"youtube"},"openai.com":{"group":"ai"}}`)
		}
	})
}

func TestRunGetDomains(t *testing.T) {
	server := youtubeServer(t)
	result := execute(t, "get", "youtube", "--type", "domains", "--url", server.URL)
	assert.Equal(t, commandResult{code: 0, out: "youtube.com\n"}, result)
}

func TestRunGetPartialIPs(t *testing.T) {
	server := youtubeServer(t)
	result := execute(t, "get", "youtube", "--type", "ips", "--family", "all", "--info", "--url", server.URL)
	assert.Zero(t, result.code)
	assert.Equal(t, "1.2.3.4\n", result.out)
	assert.Contains(t, result.err, "503")
}

func TestRunGetFailedIPv6(t *testing.T) {
	server := youtubeServer(t)
	result := execute(t, "get", "youtube", "--type", "ips", "--family", "ipv6", "--url", server.URL)
	assert.Equal(t, 1, result.code)
	assert.Empty(t, result.out)
	assert.Contains(t, result.err, "error:")
	assert.NotContains(t, result.err, "warning:")
}

func TestRunServicesList(t *testing.T) {
	server := youtubeServer(t)
	result := execute(t, "services", "list", "--url", server.URL)
	assert.Equal(t, commandResult{out: "openai.com\nyoutube.com\n"}, result)
}

func TestRunServicesSearchNormalizesCase(t *testing.T) {
	server := youtubeServer(t)
	result := execute(t, "services", "search", "OPENAI", "--url", server.URL)
	assert.Equal(t, commandResult{out: "openai.com\n"}, result)
}

func TestRunServicesSearchMissingTerm(t *testing.T) {
	server := youtubeServer(t)
	result := execute(t, "services", "search", "missing", "--url", server.URL)
	assert.Equal(t, commandResult{}, result)
}

func TestRunGetExactDomain(t *testing.T) {
	server := youtubeServer(t)
	result := execute(t, "get", "youtube.com", "--type", "domains", "--url", server.URL)
	assert.Equal(t, commandResult{out: "youtube.com\n"}, result)
}

func TestRunGetSite(t *testing.T) {
	server := youtubeServer(t)
	result := execute(t, "get", "youtube.com", "--site", "--type", "domains", "--url", server.URL)
	assert.Equal(t, commandResult{out: "youtube.com\n"}, result)
}

func telegramServer(t *testing.T, dataCalls *atomic.Int32) *httptest.Server {
	t.Helper()
	return testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("data") == "" {
			writeResponse(t, w, `{"telegram.org":{"group":"messengers"},"telegram.org.example.com":{"group":"messengers"},"whatsapp.com":{"group":"messengers"}}`)
			return
		}
		dataCalls.Add(1)
		assert.Equal(t, "telegram.org", r.URL.Query().Get("site"))
		assert.Empty(t, r.URL.Query().Get("group"))
		writeResponse(t, w, `{"telegram.org":["192.0.2.1"]}`)
	})
}

func TestRunSearchShowsSeparatePortals(t *testing.T) {
	var calls atomic.Int32
	server := telegramServer(t, &calls)
	result := execute(t, "services", "search", "telegram", "--url", server.URL)
	assert.Equal(t, commandResult{out: "telegram.org\ntelegram.org.example.com\n"}, result)
	assert.Zero(t, calls.Load())
}

func TestRunSearchIgnoresSourceGroups(t *testing.T) {
	var calls atomic.Int32
	server := telegramServer(t, &calls)
	result := execute(t, "services", "search", "messengers", "--url", server.URL)
	assert.Equal(t, commandResult{}, result)
	assert.Zero(t, calls.Load())
}

func TestRunGetAmbiguousServiceDoesNotFetchData(t *testing.T) {
	var calls atomic.Int32
	server := telegramServer(t, &calls)
	result := execute(t, "get", "tele", "--type", "ips", "--url", server.URL)
	assert.Equal(t, 2, result.code)
	assert.Empty(t, result.out)
	assert.Contains(t, result.err, "ambiguous")
	assert.Zero(t, calls.Load())
}

func TestRunGetExactServiceFetchesOnce(t *testing.T) {
	var calls atomic.Int32
	server := telegramServer(t, &calls)
	result := execute(t, "get", "telegram.org", "--type", "ips", "--url", server.URL)
	assert.Equal(t, commandResult{out: "192.0.2.1\n"}, result)
	assert.Equal(t, int32(1), calls.Load())
}

func TestRunGetMissingServiceDoesNotFetchData(t *testing.T) {
	var calls atomic.Int32
	server := telegramServer(t, &calls)
	result := execute(t, "get", "missing", "--type", "ips", "--url", server.URL)
	assert.Equal(t, 2, result.code)
	assert.Contains(t, result.err, "no portal matching")
	assert.Zero(t, calls.Load())
}

func failedCatalogueServer(t *testing.T) *httptest.Server {
	t.Helper()
	return testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("data") == "" {
			http.Error(w, "catalogue unavailable", http.StatusServiceUnavailable)
			return
		}
		writeResponse(t, w, `{"telegram.org":["192.0.2.1"]}`)
	})
}

func TestRunListReportsCatalogueFailure(t *testing.T) {
	server := failedCatalogueServer(t)
	result := execute(t, "services", "list", "--url", server.URL)
	assert.Equal(t, 1, result.code)
	assert.Empty(t, result.out)
	assert.Contains(t, result.err, "error:")
	assert.NotContains(t, result.err, "warning:")
}

func TestRunGetReportsCatalogueFailure(t *testing.T) {
	server := failedCatalogueServer(t)
	result := execute(t, "get", "telegram", "--type", "ips", "--url", server.URL)
	assert.Equal(t, 1, result.code)
	assert.Empty(t, result.out)
	assert.Contains(t, result.err, "catalogue")
}

func TestRunSiteBypassesFailedCatalogue(t *testing.T) {
	server := failedCatalogueServer(t)
	result := execute(t, "get", "telegram.org", "--site", "--type", "ips", "--url", server.URL)
	assert.Equal(t, commandResult{out: "192.0.2.1\n"}, result)
}

func assertInvalid(t *testing.T, args ...string) {
	t.Helper()
	result := execute(t, args...)
	assert.Equal(t, 2, result.code)
	assert.Empty(t, result.out)
	assert.NotEmpty(t, result.err)
}

func TestRunRejectsMissingService(t *testing.T) { assertInvalid(t, "get") }
func TestRunRejectsMissingType(t *testing.T)    { assertInvalid(t, "get", "youtube") }
func TestRunRejectsUnknownType(t *testing.T)    { assertInvalid(t, "get", "youtube", "--type", "bad") }
func TestRunRejectsFamilyForDomains(t *testing.T) {
	assertInvalid(t, "get", "youtube", "--type", "domains", "--family", "ipv4")
}
func TestRunRejectsUnknownFamily(t *testing.T) {
	assertInvalid(t, "get", "youtube", "--type", "ips", "--family", "bad")
}
func TestRunRejectsZeroTimeout(t *testing.T) {
	assertInvalid(t, "get", "youtube", "--type", "ips", "--timeout", "0")
}
func TestRunRejectsInvalidURL(t *testing.T) {
	assertInvalid(t, "get", "youtube", "--type", "ips", "--url", "://bad")
}
func TestRunRejectsGetJSONFormat(t *testing.T) {
	assertInvalid(t, "get", "youtube", "--type", "ips", "--format", "json")
}
func TestRunRejectsGetTextFormat(t *testing.T) {
	assertInvalid(t, "get", "youtube", "--type", "domains", "--format", "text")
}
func TestRunRejectsListFormat(t *testing.T) { assertInvalid(t, "services", "list", "--format", "json") }
func TestRunRejectsSearchFormat(t *testing.T) {
	assertInvalid(t, "services", "search", "youtube", "--format", "text")
}
func TestRunRejectsEmptyService(t *testing.T) { assertInvalid(t, "get", " ", "--type", "ips") }
func TestRunRejectsExtraGetArgument(t *testing.T) {
	assertInvalid(t, "get", "youtube", "--type", "ips", "extra")
}
func TestRunRejectsMissingSearchTerm(t *testing.T)      { assertInvalid(t, "services", "search") }
func TestRunRejectsUnknownServicesCommand(t *testing.T) { assertInvalid(t, "services", "bad") }
func TestRunRejectsUnknownCommand(t *testing.T)         { assertInvalid(t, "unknown") }

func TestRunNoArgumentsShowsHelp(t *testing.T) {
	result := execute(t)
	assert.Zero(t, result.code)
	assert.Contains(t, result.out, "netCatalog")
	assert.Empty(t, result.err)
}

func TestRunSourcesList(t *testing.T) {
	result := execute(t, "sources", "list")
	assert.Equal(t, commandResult{out: "iplist\tdomains,ips,cidrs\nv2fly\tdomains\n"}, result)
}

func TestRunGetHelpWithService(t *testing.T) {
	result := execute(t, "get", "youtube", "--help")
	assert.Zero(t, result.code)
	assert.NotContains(t, result.out+result.err, "--format")
}

func discordServer(t *testing.T, calls chan string) *httptest.Server {
	t.Helper()
	return testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("data") == "" {
			writeResponse(t, w, `{"discord.com":{},"discord.gg":{},"discord.media":{},"discord-tools.com":{},"notdiscord.com":{}}`)
			return
		}
		site := r.URL.Query().Get("site")
		calls <- site
		assert.Empty(t, r.URL.Query().Get("group"))
		if site == "discord.media" {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		values := map[string]map[string]string{
			"domains": {"discord.com": "cdn.discord.com", "discord.gg": "discord.gg"},
			"ip4":     {"discord.com": "192.0.2.1", "discord.gg": "192.0.2.2"},
			"cidr4":   {"discord.com": "192.0.2.0/24", "discord.gg": "198.51.100.0/24"},
		}
		value := values[r.URL.Query().Get("data")][site]
		assert.NotEmpty(t, value)
		writeResponse(t, w, fmt.Sprintf(`{%q:[%q,%q]}`, site, value, value))
	})
}

func assertCombinedDiscord(t *testing.T, kind, want string) {
	t.Helper()
	calls := make(chan string, 10)
	server := discordServer(t, calls)
	result := execute(t, "get", "discord", "--type", kind, "--info", "--url", server.URL)
	assert.Zero(t, result.code)
	assert.Equal(t, want, result.out)
	assert.Contains(t, result.err, "503")
	assert.Len(t, calls, 3)
	if len(calls) == 3 {
		assert.Equal(t, "discord.com", <-calls)
		assert.Equal(t, "discord.gg", <-calls)
		assert.Equal(t, "discord.media", <-calls)
	}
}

func TestRunCombinesDiscordDomains(t *testing.T) {
	assertCombinedDiscord(t, "domains", "cdn.discord.com\ndiscord.gg\n")
}
func TestRunCombinesDiscordIPs(t *testing.T) {
	assertCombinedDiscord(t, "ips", "192.0.2.1\n192.0.2.2\n")
}
func TestRunCombinesDiscordCIDRs(t *testing.T) {
	assertCombinedDiscord(t, "cidrs", "192.0.2.0/24\n198.51.100.0/24\n")
}

func assertExactDiscord(t *testing.T, kind string) {
	t.Helper()
	calls := make(chan string, 10)
	server := discordServer(t, calls)
	result := execute(t, "get", "discord.com", "--type", kind, "--url", server.URL)
	assert.Zero(t, result.code)
	assert.Len(t, calls, 1)
	if len(calls) == 1 {
		assert.Equal(t, "discord.com", <-calls)
	}
}

func TestRunExactDiscordDomainsDoesNotBroaden(t *testing.T) { assertExactDiscord(t, "domains") }
func TestRunExactDiscordIPsDoesNotBroaden(t *testing.T)     { assertExactDiscord(t, "ips") }
func TestRunExactDiscordCIDRsDoesNotBroaden(t *testing.T)   { assertExactDiscord(t, "cidrs") }

func warningServer(t *testing.T) *httptest.Server {
	t.Helper()
	return testServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("data") {
		case "":
			writeResponse(t, w, `{"telegram.org":{},"invalid@key":{}}`)
		case "domains":
			writeResponse(t, w, `{"telegram.org":["telegram.org","bad domain"]}`)
		case "ip4":
			writeResponse(t, w, `{"telegram.org":["192.0.2.1"]}`)
		case "ip6":
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		}
	})
}

func assertWarningOutput(t *testing.T, want string, info bool, command ...string) {
	t.Helper()
	server := warningServer(t)
	args := append(command, "--url", server.URL)
	if info {
		args = append(args, "--info")
	}
	result := execute(t, args...)
	assert.Zero(t, result.code)
	assert.Equal(t, want, result.out)
	if info {
		assert.Contains(t, result.err, "warning:")
		assert.NotContains(t, result.err, "\x1b")
	} else {
		assert.Empty(t, result.err)
	}
}

func TestRunDomainWarningsHiddenByDefault(t *testing.T) {
	assertWarningOutput(t, "telegram.org\n", false, "get", "telegram", "--type", "domains")
}
func TestRunDomainWarningsEnabledByInfo(t *testing.T) {
	assertWarningOutput(t, "telegram.org\n", true, "get", "telegram", "--type", "domains")
}
func TestRunIPWarningsHiddenByDefault(t *testing.T) {
	assertWarningOutput(t, "192.0.2.1\n", false, "get", "telegram", "--type", "ips", "--family", "all")
}
func TestRunIPWarningsEnabledByInfo(t *testing.T) {
	assertWarningOutput(t, "192.0.2.1\n", true, "get", "telegram", "--type", "ips", "--family", "all")
}
func TestRunListWarningsHiddenByDefault(t *testing.T) {
	assertWarningOutput(t, "telegram.org\n", false, "services", "list")
}
func TestRunListWarningsEnabledByInfo(t *testing.T) {
	assertWarningOutput(t, "telegram.org\n", true, "services", "list")
}
func TestRunSearchWarningsHiddenByDefault(t *testing.T) {
	assertWarningOutput(t, "telegram.org\n", false, "services", "search", "telegram")
}
func TestRunSearchWarningsEnabledByInfo(t *testing.T) {
	assertWarningOutput(t, "telegram.org\n", true, "services", "search", "telegram")
}

func TestRunSiteFailureAlwaysReportsError(t *testing.T) {
	server := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	})
	result := execute(t, "get", "telegram.org", "--site", "--type", "domains", "--url", server.URL)
	assert.Equal(t, 1, result.code)
	assert.Empty(t, result.out)
	assert.Contains(t, result.err, "error:")
	assert.NotContains(t, result.err, "warning:")
}

const release = `lists:
  - name: discord
    length: 4
    rules: ["domain:shared.com", "domain:cdn.discord.com:@cn", "full:ignored.discord.com", "keyword:discord"]
  - name: standalone
    length: 1
    rules: ["domain:standalone.org"]
  - name: categorymessengers
    length: 1
    rules: ["domain:excluded.com"]
  - name: discord-extra
    length: 1
    rules: ["domain:excluded.net"]
`

func combinedSources(t *testing.T, iplistDown, v2flyDown bool, downloads *atomic.Int32) (*httptest.Server, *httptest.Server) {
	t.Helper()
	v2flyServer := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		downloads.Add(1)
		if v2flyDown {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		writeResponse(t, w, release)
	})
	iplistServer := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if iplistDown {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		if r.URL.Query().Get("data") == "" {
			writeResponse(t, w, `{"discord.com":{},"discord.gg":{}}`)
			return
		}
		if r.URL.Query().Get("site") == "discord.gg" {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		writeResponse(t, w, `{"discord.com":["shared.com"]}`)
	})
	return iplistServer, v2flyServer
}

func assertV2flyIntegration(t *testing.T, term, want string, iplistDown, v2flyDown bool) {
	t.Helper()
	var downloads atomic.Int32
	iplistServer, v2flyServer := combinedSources(t, iplistDown, v2flyDown, &downloads)
	var out, diagnostics bytes.Buffer
	code := Run(context.Background(), []string{"get", term, "--type", "domains", "--url", iplistServer.URL, "--v2fly-url", v2flyServer.URL}, &out, &diagnostics)
	assert.Zero(t, code)
	assert.Equal(t, want, out.String())
	assert.Empty(t, diagnostics.String())
	assert.Equal(t, int32(1), downloads.Load())
}

func TestRunCombinesBothSources(t *testing.T) {
	assertV2flyIntegration(t, "discord", "cdn.discord.com\nshared.com\n", false, false)
}
func TestRunExactPortalExcludesNamedList(t *testing.T) {
	assertV2flyIntegration(t, "discord.com", "shared.com\n", false, false)
}
func TestRunFetchesStandaloneList(t *testing.T) {
	assertV2flyIntegration(t, "standalone", "standalone.org\n", false, false)
}
func TestRunRetainsV2flyWhenIplistFails(t *testing.T) {
	assertV2flyIntegration(t, "discord", "cdn.discord.com\nshared.com\n", true, false)
}
func TestRunRetainsIplistWhenV2flyFails(t *testing.T) {
	assertV2flyIntegration(t, "discord", "shared.com\n", false, true)
}

func assertV2flyNotRequested(t *testing.T, command ...string) {
	t.Helper()
	var downloads atomic.Int32
	v2flyServer := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		downloads.Add(1)
		http.Error(w, "must not be called", http.StatusServiceUnavailable)
	})
	iplistServer := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("data") {
		case "":
			writeResponse(t, w, `{"telegram.org":{}}`)
		case "ip4":
			writeResponse(t, w, `{"telegram.org":["192.0.2.1"]}`)
		case "cidr4":
			writeResponse(t, w, `{"telegram.org":["192.0.2.0/24"]}`)
		case "domains":
			writeResponse(t, w, `{"telegram.org":["telegram.org"]}`)
		}
	})
	args := append(command, "--url", iplistServer.URL, "--v2fly-url", v2flyServer.URL)
	result := execute(t, args...)
	assert.Zero(t, result.code)
	assert.NotEmpty(t, result.out)
	assert.Empty(t, result.err)
	assert.Zero(t, downloads.Load())
}

func TestRunIPsDoesNotContactV2fly(t *testing.T) {
	assertV2flyNotRequested(t, "get", "telegram", "--type", "ips")
}
func TestRunCIDRsDoesNotContactV2fly(t *testing.T) {
	assertV2flyNotRequested(t, "get", "telegram", "--type", "cidrs")
}
func TestRunSiteDoesNotContactV2fly(t *testing.T) {
	assertV2flyNotRequested(t, "get", "telegram.org", "--type", "domains", "--site")
}

func TestRunSearchCombinedCatalogue(t *testing.T) {
	server := testServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/release" {
			writeResponse(t, w, "lists:\n  - name: telegram\n    length: 1\n    rules: [\"domain:telegram.org\"]\n  - name: categoryall\n    length: 0\n    rules: []\n")
		} else {
			writeResponse(t, w, `{"telegram.org":{}}`)
		}
	})
	result := execute(t, "services", "search", "telegram", "--url", server.URL, "--v2fly-url", server.URL+"/release")
	assert.Equal(t, commandResult{out: "telegram\ntelegram.org\n"}, result)
}
