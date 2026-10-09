package app_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	code := runForTest(context.Background(), t, args, &out, &diagnostics, "")
	return commandResult{code: code, out: out.String(), err: diagnostics.String()}
}

func testServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(handler)
	t.Cleanup(s.Close)
	return s
}

func fixtureDir(t *testing.T, iplist, v2fly string) string {
	t.Helper()
	dir := t.TempDir()
	if iplist != "" {
		seedSnapshot(t, dir, "iplist", iplist)
	}
	if v2fly != "" {
		seedSnapshot(t, dir, "v2fly", v2fly)
	}
	return dir
}

func executeCatalog(t *testing.T, payload string, args ...string) commandResult {
	t.Helper()
	return execute(t, append(args, "--data-dir", fixtureDir(t, payload, ""))...)
}

const youtubeFixture = `{"youtube.com":{"group":"youtube","domains":["YouTube.com","*.youtube.com","bad domain"],"ip4":["1.2.3.4","invalid"],"ip6":[],"cidr4":[],"cidr6":[]},"openai.com":{"group":"ai","domains":[],"ip4":[],"ip6":[],"cidr4":[],"cidr6":[]}}`
const telegramFixture = `{"telegram.org":{"group":"messengers","domains":["telegram.org"],"ip4":["192.0.2.1"],"ip6":[],"cidr4":["192.0.2.0/24"],"cidr6":[]},"telegram.org.example.com":{"group":"messengers","domains":[],"ip4":[],"ip6":[],"cidr4":[],"cidr6":[]},"whatsapp.com":{"group":"messengers","domains":[],"ip4":[],"ip6":[],"cidr4":[],"cidr6":[]}}`
const warningFixture = `{"telegram.org":{"domains":["telegram.org","bad domain"],"ip4":["192.0.2.1","invalid"],"ip6":[],"cidr4":[],"cidr6":[]},"invalid@key":{}}`
const discordFixture = `{"discord.com":{"domains":["cdn.discord.com","cdn.discord.com"],"ip4":["192.0.2.1","192.0.2.1"],"ip6":[],"cidr4":["192.0.2.0/24"],"cidr6":[]},"discord.gg":{"domains":["discord.gg"],"ip4":["192.0.2.2"],"ip6":[],"cidr4":["198.51.100.0/24"],"cidr6":[]},"discord.media":{"domains":[],"ip4":[],"ip6":[],"cidr4":[],"cidr6":[]},"discord-tools.com":{"domains":["unrelated.org"],"ip4":["203.0.113.1"],"ip6":[],"cidr4":["203.0.113.0/24"],"cidr6":[]},"notdiscord.com":{"domains":["unrelated.net"],"ip4":[],"ip6":[],"cidr4":[],"cidr6":[]}}`
const sharedFixture = `{"discord.com":{"domains":["shared.com"],"ip4":[],"ip6":[],"cidr4":[],"cidr6":[]},"discord.gg":{"domains":[],"ip4":[],"ip6":[],"cidr4":[],"cidr6":[]}}`
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

func TestRunGetDomains(t *testing.T) {
	assert.Equal(t, commandResult{out: "youtube.com\n"}, executeCatalog(t, youtubeFixture, "get", "youtube", "--type", "domains"))
}
func TestRunGetPartialIPs(t *testing.T) {
	r := executeCatalog(t, youtubeFixture, "get", "youtube", "--type", "ips", "--family", "all", "--info")
	assert.Zero(t, r.code)
	assert.Equal(t, "1.2.3.4\n", r.out)
	assert.Contains(t, r.err, "invalid records")
}
func TestRunGetEmptyIPv6Succeeds(t *testing.T) {
	assert.Equal(t, commandResult{}, executeCatalog(t, youtubeFixture, "get", "youtube", "--type", "ips", "--family", "ipv6"))
}
func TestRunServicesList(t *testing.T) {
	assert.Equal(t, commandResult{out: "openai.com\nyoutube.com\n"}, executeCatalog(t, youtubeFixture, "services", "list"))
}
func TestRunServicesSearchNormalizesCase(t *testing.T) {
	assert.Equal(t, commandResult{out: "openai.com\n"}, executeCatalog(t, youtubeFixture, "services", "search", "OPENAI"))
}
func TestRunServicesSearchMissingTerm(t *testing.T) {
	assert.Equal(t, commandResult{}, executeCatalog(t, youtubeFixture, "services", "search", "missing"))
}
func TestRunGetExactDomain(t *testing.T) {
	assert.Equal(t, commandResult{out: "youtube.com\n"}, executeCatalog(t, youtubeFixture, "get", "youtube.com", "--type", "domains"))
}
func TestRunSearchShowsSeparatePortals(t *testing.T) {
	assert.Equal(t, commandResult{out: "telegram.org\ntelegram.org.example.com\n"}, executeCatalog(t, telegramFixture, "services", "search", "telegram"))
}
func TestRunSearchIgnoresSourceGroups(t *testing.T) {
	assert.Equal(t, commandResult{}, executeCatalog(t, telegramFixture, "services", "search", "messengers"))
}
func TestRunGetAmbiguousServiceDoesNotFetchData(t *testing.T) {
	tr := forbidHTTP(t)
	r := executeCatalog(t, telegramFixture, "get", "tele", "--type", "ips")
	assert.Equal(t, 2, r.code)
	assert.Empty(t, r.out)
	assert.Contains(t, r.err, "ambiguous")
	assert.Zero(t, tr.calls.Load())
}
func TestRunGetExactServiceDoesNotBroaden(t *testing.T) {
	assert.Equal(t, commandResult{out: "192.0.2.1\n"}, executeCatalog(t, telegramFixture, "get", "telegram.org", "--type", "ips"))
}
func TestRunGetMissingServiceDoesNotFetchData(t *testing.T) {
	tr := forbidHTTP(t)
	r := executeCatalog(t, telegramFixture, "get", "missing", "--type", "ips")
	assert.Equal(t, 2, r.code)
	assert.Contains(t, r.err, "no portal matching")
	assert.Zero(t, tr.calls.Load())
}
func TestRunListReportsCatalogueFailure(t *testing.T) {
	r := execute(t, "services", "list")
	assert.Equal(t, 1, r.code)
	assert.Empty(t, r.out)
	assert.Contains(t, r.err, "netCatalog update")
	assert.NotContains(t, r.err, "warning:")
}
func TestRunGetReportsCatalogueFailure(t *testing.T) {
	r := execute(t, "get", "telegram", "--type", "ips")
	assert.Equal(t, 1, r.code)
	assert.Empty(t, r.out)
	assert.Contains(t, r.err, "catalogue")
}

func assertInvalid(t *testing.T, args ...string) {
	t.Helper()
	r := execute(t, args...)
	assert.Equal(t, 2, r.code)
	assert.Empty(t, r.out)
	assert.NotEmpty(t, r.err)
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
func TestRunRejectsZeroTimeout(t *testing.T) { assertInvalid(t, "update", "--timeout", "0s") }
func TestRunRejectsInvalidURL(t *testing.T) {
	assertInvalid(t, "update", "--source", "iplist", "--iplist-url", "://bad")
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
	r := execute(t)
	assert.Zero(t, r.code)
	assert.Contains(t, r.out, "netCatalog")
	assert.Empty(t, r.err)
}
func TestRunSourcesList(t *testing.T) {
	assert.Equal(t, commandResult{out: "iplist\tdomains,ips,cidrs\nv2fly\tdomains\n"}, execute(t, "sources", "list"))
}
func TestRunGetHelpWithService(t *testing.T) {
	r := execute(t, "get", "youtube", "--help")
	assert.Zero(t, r.code)
	assert.NotContains(t, r.out+r.err, "--format")
}

func assertCombinedDiscord(t *testing.T, kind, want string) {
	t.Helper()
	r := executeCatalog(t, discordFixture, "get", "discord", "--type", kind)
	assert.Equal(t, commandResult{out: want}, r)
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
func assertExactDiscord(t *testing.T, kind, want string) {
	t.Helper()
	assert.Equal(t, commandResult{out: want}, executeCatalog(t, discordFixture, "get", "discord.com", "--type", kind))
}
func TestRunExactDiscordDomainsDoesNotBroaden(t *testing.T) {
	assertExactDiscord(t, "domains", "cdn.discord.com\n")
}
func TestRunExactDiscordIPsDoesNotBroaden(t *testing.T) { assertExactDiscord(t, "ips", "192.0.2.1\n") }
func TestRunExactDiscordCIDRsDoesNotBroaden(t *testing.T) {
	assertExactDiscord(t, "cidrs", "192.0.2.0/24\n")
}

func assertWarningOutput(t *testing.T, want string, info bool, args ...string) {
	t.Helper()
	if info {
		args = append(args, "--info")
	}
	r := executeCatalog(t, warningFixture, args...)
	assert.Zero(t, r.code)
	assert.Equal(t, want, r.out)
	if info {
		assert.Contains(t, r.err, "warning:")
		assert.NotContains(t, r.err, "\x1b")
	} else {
		assert.Empty(t, r.err)
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

func assertV2flyIntegration(t *testing.T, term, want string, iplistDown, v2flyDown bool) {
	t.Helper()
	i, v := sharedFixture, release
	if iplistDown {
		i = ""
	}
	if v2flyDown {
		v = ""
	}
	dir := fixtureDir(t, i, v)
	tr := forbidHTTP(t)
	assert.Equal(t, commandResult{out: want}, execute(t, "get", term, "--type", "domains", "--data-dir", dir))
	assert.Zero(t, tr.calls.Load())
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
func assertV2flyNotRequested(t *testing.T, args ...string) {
	t.Helper()
	tr := forbidHTTP(t)
	r := executeCatalog(t, telegramFixture, args...)
	assert.Zero(t, r.code)
	assert.NotEmpty(t, r.out)
	assert.Empty(t, r.err)
	assert.Zero(t, tr.calls.Load())
}
func TestRunIPsDoesNotContactV2fly(t *testing.T) {
	assertV2flyNotRequested(t, "get", "telegram", "--type", "ips")
}
func TestRunCIDRsDoesNotContactV2fly(t *testing.T) {
	assertV2flyNotRequested(t, "get", "telegram", "--type", "cidrs")
}
func TestRunSelectedIPListDoesNotContactV2fly(t *testing.T) {
	assertV2flyNotRequested(t, "get", "telegram.org", "--type", "domains", "--source", "iplist")
}
func TestRunSearchCombinedCatalogue(t *testing.T) {
	dir := fixtureDir(t, telegramFixture, "lists:\n  - name: telegram\n    length: 1\n    rules: [\"domain:telegram.org\"]\n  - name: categoryall\n    length: 0\n    rules: []\n")
	assert.Equal(t, commandResult{out: "telegram\ntelegram.org\ntelegram.org.example.com\n"}, execute(t, "services", "search", "telegram", "--data-dir", dir))
}

func TestRunRejectsRemovedSiteFlag(t *testing.T) {
	r := executeCatalog(t, youtubeFixture, "get", "youtube.com", "--site", "--type", "domains")
	assert.Equal(t, 2, r.code)
	assert.Empty(t, r.out)
	assert.Contains(t, r.err, "unknown flag: --site")
}
