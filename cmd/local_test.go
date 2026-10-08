package cmd

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type forbiddenTransport struct{ calls atomic.Int32 }

func (tr *forbiddenTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	tr.calls.Add(1)
	return nil, errors.New("unexpected HTTP request during local read")
}

func forbidHTTP(t *testing.T) *forbiddenTransport {
	t.Helper()
	old := http.DefaultTransport
	tr := &forbiddenTransport{}
	http.DefaultTransport = tr
	t.Cleanup(func() { http.DefaultTransport = old })
	return tr
}

func updatedLocalDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	i := testServer(t, func(w http.ResponseWriter, _ *http.Request) { writeResponse(t, w, fullIPList) })
	v := testServer(t, func(w http.ResponseWriter, _ *http.Request) { writeResponse(t, w, release) })
	r := execute(t, "update", "--data-dir", dir, "--iplist-url", i.URL, "--v2fly-url", v.URL)
	require.Zero(t, r.code, r.err)
	i.Close()
	v.Close()
	return dir
}

func assertLocalRead(t *testing.T, want string, args ...string) {
	t.Helper()
	dir := updatedLocalDir(t)
	tr := forbidHTTP(t)
	r := execute(t, append(args, "--data-dir", dir, "--iplist-url", ":invalid", "--v2fly-url", ":invalid", "--timeout", "0s")...)
	assert.Equal(t, commandResult{out: want}, r)
	assert.Zero(t, tr.calls.Load())
}

func TestRunLocalDomains(t *testing.T) {
	assertLocalRead(t, "cdn.discord.com\niplist.example\nshared.com\n", "get", "discord", "--type", "domains")
}

func TestRunLocalIPs(t *testing.T) {
	assertLocalRead(t, "192.0.2.1\n2001:db8::1\n", "get", "discord", "--type", "ips", "--family", "all")
}

func TestRunLocalCIDRs(t *testing.T) {
	assertLocalRead(t, "192.0.2.0/24\n2001:db8::/64\n", "get", "discord", "--type", "cidrs", "--family", "all")
}

func TestRunLocalSite(t *testing.T) {
	assertLocalRead(t, "iplist.example\n", "get", "discord.com", "--site", "--type", "domains")
}

func TestRunLocalList(t *testing.T) {
	assertLocalRead(t, "discord\ndiscord.com\nstandalone\n", "services", "list")
}

func TestRunLocalSearch(t *testing.T) {
	assertLocalRead(t, "discord\ndiscord.com\n", "services", "search", "DISCORD")
}

func TestRunLocalMissingSnapshots(t *testing.T) {
	tr := forbidHTTP(t)
	r := execute(t, "services", "list", "--data-dir", t.TempDir())
	assert.Equal(t, 1, r.code)
	assert.Contains(t, r.err, "netCatalog update")
	assert.Zero(t, tr.calls.Load())
}

func TestRunLocalSelectedMissingSource(t *testing.T) {
	dir := t.TempDir()
	seedSnapshot(t, dir, "iplist", fullIPList)
	r := execute(t, "services", "list", "--source", "v2fly", "--data-dir", dir)
	assert.Equal(t, 1, r.code)
	assert.Contains(t, r.err, "netCatalog update --source v2fly")
}

func TestRunLocalPartialCatalogHidesWarnings(t *testing.T) {
	dir := t.TempDir()
	seedSnapshot(t, dir, "iplist", fullIPList)
	r := execute(t, "services", "list", "--data-dir", dir)
	assert.Equal(t, commandResult{out: "discord.com\n"}, r)
}

func TestRunLocalPartialCatalogReportsWarningsWithInfo(t *testing.T) {
	dir := t.TempDir()
	seedSnapshot(t, dir, "iplist", fullIPList)
	r := execute(t, "services", "list", "--data-dir", dir, "--info")
	assert.Zero(t, r.code)
	assert.Equal(t, "discord.com\n", r.out)
	assert.Contains(t, r.err, "netCatalog update --source v2fly")
}

func TestRunLocalIPsIgnoreCorruptedV2fly(t *testing.T) {
	dir := t.TempDir()
	seedSnapshot(t, dir, "iplist", fullIPList)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "v2fly.snapshot"), []byte("broken"), 0600))
	r := execute(t, "get", "discord", "--type", "ips", "--data-dir", dir, "--info")
	assert.Equal(t, commandResult{out: "192.0.2.1\n"}, r)
}

func TestRunLocalRejectsUnknownSite(t *testing.T) {
	dir := t.TempDir()
	seedSnapshot(t, dir, "iplist", fullIPList)
	r := execute(t, "get", "missing.org", "--site", "--type", "domains", "--data-dir", dir)
	assert.Equal(t, 2, r.code)
	assert.Empty(t, r.out)
}

func TestRunLocalDoesNotRetainDirectoryOrSource(t *testing.T) {
	first := t.TempDir()
	seedSnapshot(t, first, "v2fly", release)
	assert.Equal(t, commandResult{out: "discord\nstandalone\n"}, execute(t, "services", "list", "--source", "v2fly", "--data-dir", first))
	second := t.TempDir()
	seedSnapshot(t, second, "iplist", fullIPList)
	assert.Equal(t, commandResult{out: "discord.com\n"}, execute(t, "services", "list", "--data-dir", second))
}

func TestRunLocalRejectsCorruptedSelectedSnapshot(t *testing.T) {
	dir := t.TempDir()
	seedSnapshot(t, dir, "v2fly", "broken: true")
	tr := forbidHTTP(t)
	r := execute(t, "services", "list", "--source", "v2fly", "--data-dir", dir)
	assert.Equal(t, 1, r.code)
	assert.Contains(t, r.err, "netCatalog update --source v2fly")
	assert.Zero(t, tr.calls.Load())
}

func TestRunLocalSitePreservesPortalKey(t *testing.T) {
	dir := t.TempDir()
	seedSnapshot(t, dir, "iplist", `{"TELEGRAM.ORG.":{"domains":["telegram.org"],"ip4":[],"ip6":[],"cidr4":[],"cidr6":[]}}`)
	assert.Equal(t, commandResult{out: "telegram.org\n"}, execute(t, "get", "TELEGRAM.ORG.", "--site", "--type", "domains", "--data-dir", dir))
}

func TestRunHelpDocumentsOfflineWorkflow(t *testing.T) {
	tr := forbidHTTP(t)
	r := execute(t, "--help", "--data-dir", filepath.Join(t.TempDir(), "absent"))
	assert.Zero(t, r.code)
	assert.Contains(t, r.out, "netCatalog update")
	assert.Contains(t, r.out, "local")
	assert.Contains(t, r.out, "--data-dir")
	assert.Contains(t, r.out, "--source")
	assert.Zero(t, tr.calls.Load())
}

func TestRunUpdateHelpDocumentsIndependentSnapshots(t *testing.T) {
	tr := forbidHTTP(t)
	r := execute(t, "update", "--help")
	assert.Zero(t, r.code)
	assert.Contains(t, r.out, "--source")
	assert.Contains(t, r.out, "--data-dir")
	assert.Contains(t, r.out, "previous snapshot")
	assert.Zero(t, tr.calls.Load())
}

func TestRunGetHelpDocumentsLocalReads(t *testing.T) {
	tr := forbidHTTP(t)
	r := execute(t, "get", "--help")
	assert.Zero(t, r.code)
	assert.Contains(t, r.out, "local")
	assert.Contains(t, r.out, "netCatalog update")
	assert.Zero(t, tr.calls.Load())
}

func TestRunSourcesWithoutDataDirectory(t *testing.T) {
	tr := forbidHTTP(t)
	r := execute(t, "sources", "list", "--data-dir", filepath.Join(t.TempDir(), "absent"), "--source", "v2fly")
	assert.Equal(t, commandResult{out: "iplist\tdomains,ips,cidrs\nv2fly\tdomains\n"}, r)
	assert.Zero(t, tr.calls.Load())
}
