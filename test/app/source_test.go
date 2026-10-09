package app_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func assertSourceCommand(t *testing.T, source, want string, args ...string) {
	t.Helper()
	dir := fixtureDir(t, fullIPList, release)
	tr := forbidHTTP(t)
	r := execute(t, append(args, "--source", source, "--data-dir", dir, "--iplist-url", ":invalid", "--v2fly-url", ":invalid")...)
	assert.Equal(t, commandResult{out: want}, r)
	assert.Zero(t, tr.calls.Load())
}

func TestRunSourceFiltersGetIPList(t *testing.T) {
	assertSourceCommand(t, "iplist", "iplist.example\n", "get", "discord", "--type", "domains")
}
func TestRunSourceFiltersGetV2fly(t *testing.T) {
	assertSourceCommand(t, "v2fly", "cdn.discord.com\nshared.com\n", "get", "discord", "--type", "domains")
}
func TestRunSourceFiltersListIPList(t *testing.T) {
	assertSourceCommand(t, "iplist", "discord\n", "services", "list")
}
func TestRunSourceFiltersListV2fly(t *testing.T) {
	assertSourceCommand(t, "v2fly", "discord\nstandalone\n", "services", "list")
}
func TestRunSourceFiltersSearchIPList(t *testing.T) {
	assertSourceCommand(t, "iplist", "discord\n", "services", "search", "discord")
}
func TestRunSourceFiltersSearchV2fly(t *testing.T) {
	assertSourceCommand(t, "v2fly", "discord\n", "services", "search", "discord")
}
func TestRunSourceFiltersExactPortal(t *testing.T) {
	assertSourceCommand(t, "iplist", "iplist.example\n", "get", "discord.com", "--type", "domains")
}
func TestRunSourceFiltersIPs(t *testing.T) {
	assertSourceCommand(t, "iplist", "192.0.2.1\n", "get", "discord", "--type", "ips")
}
func TestRunSourceFiltersCIDRs(t *testing.T) {
	assertSourceCommand(t, "iplist", "192.0.2.0/24\n", "get", "discord", "--type", "cidrs")
}
func TestRunSourceRejectsUnknownGet(t *testing.T) {
	assertInvalid(t, "get", "discord", "--type", "domains", "--source", "unknown")
}
func TestRunSourceRejectsUnknownList(t *testing.T) {
	assertInvalid(t, "services", "list", "--source", "unknown")
}
func TestRunSourceRejectsUnknownSearch(t *testing.T) {
	assertInvalid(t, "services", "search", "discord", "--source", "unknown")
}
func TestRunSourceRejectsV2flyIPs(t *testing.T) {
	assertInvalid(t, "get", "discord", "--type", "ips", "--source", "v2fly")
}
func TestRunSourceRejectsV2flyCIDRs(t *testing.T) {
	assertInvalid(t, "get", "discord", "--type", "cidrs", "--source", "v2fly")
}
func TestRunSourceV2flyIgnoresEmptyURL(t *testing.T) {
	dir := fixtureDir(t, "", release)
	assert.Equal(t, commandResult{out: "discord\nstandalone\n"}, execute(t, "services", "list", "--source", "v2fly", "--v2fly-url", "", "--data-dir", dir))
}
func TestRunDoesNotRetainSourceFlag(t *testing.T) {
	dir := fixtureDir(t, sharedFixture, release)
	args := []string{"get", "discord", "--type", "domains", "--data-dir", dir}
	assert.Equal(t, commandResult{out: "shared.com\n"}, execute(t, append(args, "--source", "iplist")...))
	assert.Equal(t, commandResult{out: "cdn.discord.com\nshared.com\n"}, execute(t, args...))
}
func TestRunSourcesRemainsCompleteWithSourceFlag(t *testing.T) {
	assert.Equal(t, commandResult{out: "iplist\tdomains,ips,cidrs\t-\nv2fly\tdomains\t-\n"}, execute(t, "sources", "--source", "iplist"))
}
