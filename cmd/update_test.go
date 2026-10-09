package cmd

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"netCatalog/internal/catalogstore"
	"netCatalog/internal/providers/iplist"
	"netCatalog/internal/providers/v2fly"
)

const fullIPList = `{"discord.com":{"domains":["iplist.example"],"ip4":["192.0.2.1"],"ip6":["2001:db8::1"],"cidr4":["192.0.2.123/24"],"cidr6":["2001:db8::1/64"]}}`

func seedSnapshot(t *testing.T, dir, source, payload string) {
	t.Helper()
	require.NoError(t, catalogstore.Write(context.Background(), dir, catalogstore.Snapshot{
		Metadata: catalogstore.Metadata{Version: 1, Source: source, URL: "https://old.example/", UpdatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		Payload:  []byte(payload),
	}))
}

func payloadServer(t *testing.T, payload string, calls *atomic.Int32) string {
	t.Helper()
	s := testServer(t, func(w http.ResponseWriter, _ *http.Request) {
		if calls != nil {
			calls.Add(1)
		}
		writeResponse(t, w, payload)
	})
	return s.URL
}

func TestRunUpdateBothSources(t *testing.T) {
	dir := t.TempDir()
	var i, v atomic.Int32
	iURL := payloadServer(t, fullIPList, &i)
	vURL := payloadServer(t, release, &v)
	r := execute(t, "update", "--data-dir", dir, "--iplist-url", iURL, "--v2fly-url", vURL)
	require.Zero(t, r.code, r.err)
	assert.Empty(t, r.err)
	assert.Contains(t, r.out, "iplist")
	assert.Contains(t, r.out, "v2fly")
	assert.Contains(t, r.out, dir)
	assert.Equal(t, int32(1), i.Load())
	assert.Equal(t, int32(1), v.Load())
	is, err := catalogstore.Read(dir, "iplist")
	require.NoError(t, err)
	assert.Equal(t, iURL, is.Metadata.URL)
	assert.Equal(t, 1, is.Metadata.Version)
	assert.False(t, is.Metadata.UpdatedAt.IsZero())
	_, err = iplist.NewSnapshot(is.Payload)
	require.NoError(t, err)
	vs, err := catalogstore.Read(dir, "v2fly")
	require.NoError(t, err)
	assert.Equal(t, vURL, vs.Metadata.URL)
	_, err = v2fly.NewSnapshot(vs.Payload)
	require.NoError(t, err)
}

func TestRunUpdateOnlyIPList(t *testing.T) {
	dir := t.TempDir()
	seedSnapshot(t, dir, "v2fly", release)
	before, err := os.ReadFile(filepath.Join(dir, "v2fly.snapshot"))
	require.NoError(t, err)
	r := execute(t, "update", "--source", "iplist", "--data-dir", dir, "--iplist-url", payloadServer(t, fullIPList, nil), "--v2fly-url", ":invalid")
	require.Zero(t, r.code, r.err)
	after, err := os.ReadFile(filepath.Join(dir, "v2fly.snapshot"))
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestRunUpdateOnlyV2fly(t *testing.T) {
	dir := t.TempDir()
	seedSnapshot(t, dir, "iplist", fullIPList)
	before, err := os.ReadFile(filepath.Join(dir, "iplist.snapshot"))
	require.NoError(t, err)
	r := execute(t, "update", "--source", "v2fly", "--data-dir", dir, "--v2fly-url", payloadServer(t, release, nil), "--iplist-url", ":invalid")
	require.Zero(t, r.code, r.err)
	after, err := os.ReadFile(filepath.Join(dir, "iplist.snapshot"))
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestRunUpdateRetainsOldSnapshotOnFailure(t *testing.T) {
	dir := t.TempDir()
	seedSnapshot(t, dir, "iplist", fullIPList)
	before, err := os.ReadFile(filepath.Join(dir, "iplist.snapshot"))
	require.NoError(t, err)
	s := testServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) })
	r := execute(t, "update", "--source", "iplist", "--data-dir", dir, "--iplist-url", s.URL)
	assert.Equal(t, 1, r.code)
	assert.Contains(t, r.err, "iplist")
	assert.Contains(t, r.err, "503")
	after, err := os.ReadFile(filepath.Join(dir, "iplist.snapshot"))
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestRunUpdatePartialFailureSavesSuccessfulSource(t *testing.T) {
	dir := t.TempDir()
	s := testServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) })
	r := execute(t, "update", "--data-dir", dir, "--iplist-url", s.URL, "--v2fly-url", payloadServer(t, release, nil))
	assert.Equal(t, 1, r.code)
	assert.Contains(t, r.err, "iplist")
	_, err := catalogstore.Read(dir, "v2fly")
	require.NoError(t, err)
}

func TestRunUpdateRejectsUnknownSourceBeforeHTTP(t *testing.T) {
	var calls atomic.Int32
	u := payloadServer(t, fullIPList, &calls)
	r := execute(t, "update", "--source", "bad", "--iplist-url", u)
	assert.Equal(t, 2, r.code)
	assert.Contains(t, r.err, "iplist or v2fly")
	assert.Zero(t, calls.Load())
}

func TestRunUpdateEmptyV2flyURLDisablesDownload(t *testing.T) {
	dir := t.TempDir()
	r := execute(t, "update", "--data-dir", dir, "--iplist-url", payloadServer(t, fullIPList, nil), "--v2fly-url", "")
	require.Zero(t, r.code, r.err)
	_, err := catalogstore.Read(dir, "v2fly")
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestRunUpdateRejectsExplicitDisabledV2fly(t *testing.T) {
	r := execute(t, "update", "--source", "v2fly", "--v2fly-url", "")
	assert.Equal(t, 2, r.code)
	assert.Contains(t, r.err, "--v2fly-url")
}

func TestRunUpdateRejectsInvalidTimeout(t *testing.T) {
	var calls atomic.Int32
	r := execute(t, "update", "--source", "iplist", "--iplist-url", payloadServer(t, fullIPList, &calls), "--timeout", "0s")
	assert.Equal(t, 2, r.code)
	assert.Contains(t, r.err, "timeout")
	assert.Zero(t, calls.Load())
}

func TestRunUpdateRejectsSelectedInvalidURL(t *testing.T) {
	r := execute(t, "update", "--source", "iplist", "--iplist-url", ":invalid")
	assert.Equal(t, 2, r.code)
	assert.Contains(t, r.err, "URL")
}

func TestRunUpdateReportsWriteFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(path, []byte("unchanged"), 0600))
	r := execute(t, "update", "--source", "iplist", "--data-dir", path, "--iplist-url", payloadServer(t, fullIPList, nil))
	assert.Equal(t, 1, r.code)
	assert.Contains(t, r.err, "iplist")
}

func TestRunUpdateReportsOutputFailure(t *testing.T) {
	var diagnostics bytes.Buffer
	code := runForTest(context.Background(), t, []string{"update", "--source", "iplist", "--iplist-url", payloadServer(t, fullIPList, nil)}, failingWriter{}, &diagnostics, "")
	assert.Equal(t, 1, code)
	assert.Contains(t, diagnostics.String(), "output unavailable")
}

func TestRunUpdateReplacesSelectedURL(t *testing.T) {
	dir := t.TempDir()
	seedSnapshot(t, dir, "iplist", fullIPList)
	seedSnapshot(t, dir, "v2fly", release)
	before, err := os.ReadFile(filepath.Join(dir, "v2fly.snapshot"))
	require.NoError(t, err)
	u := payloadServer(t, `{"new.org":{"domains":[],"ip4":[],"ip6":[],"cidr4":[],"cidr6":[]}}`, nil)
	r := execute(t, "update", "--source", "iplist", "--data-dir", dir, "--iplist-url", u)
	require.Zero(t, r.code, r.err)
	s, err := catalogstore.Read(dir, "iplist")
	require.NoError(t, err)
	assert.Equal(t, u, s.Metadata.URL)
	assert.Contains(t, string(s.Payload), "new.org")
	after, err := os.ReadFile(filepath.Join(dir, "v2fly.snapshot"))
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func assertBadV2flyPreservesSnapshot(t *testing.T, payload string) {
	t.Helper()
	dir := t.TempDir()
	seedSnapshot(t, dir, "v2fly", release)
	before, err := os.ReadFile(filepath.Join(dir, "v2fly.snapshot"))
	require.NoError(t, err)
	r := execute(t, "update", "--source", "v2fly", "--data-dir", dir, "--v2fly-url", payloadServer(t, payload, nil))
	assert.Equal(t, 1, r.code)
	assert.Contains(t, r.err, "v2fly")
	after, err := os.ReadFile(filepath.Join(dir, "v2fly.snapshot"))
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestRunUpdateRejectsMissingV2flyFields(t *testing.T) {
	assertBadV2flyPreservesSnapshot(t, `lists: [{name: discord}]`)
}

func TestRunUpdateRejectsNullV2flyRules(t *testing.T) {
	assertBadV2flyPreservesSnapshot(t, `lists: [{name: discord, length: 0, rules: null}]`)
}

func TestRunUpdateRejectsNumericV2flyRules(t *testing.T) {
	assertBadV2flyPreservesSnapshot(t, `lists: [{name: discord, length: 1, rules: [123]}]`)
}

func TestRunUpdateRejectsMissingV2flyLength(t *testing.T) {
	assertBadV2flyPreservesSnapshot(t, `lists: [{name: discord, rules: []}]`)
}

func TestRunUpdateRejectsMalformedExcludedV2flyList(t *testing.T) {
	assertBadV2flyPreservesSnapshot(t, `lists: [{name: categoryall, length: 999, rules: []}]`)
}

func TestRunUpdateRejectsDuplicateExcludedV2flyNames(t *testing.T) {
	assertBadV2flyPreservesSnapshot(t, `lists: [{name: categoryall, length: 0, rules: []}, {name: categoryall, length: 0, rules: []}]`)
}
