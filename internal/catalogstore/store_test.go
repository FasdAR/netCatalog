package catalogstore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testSnapshot() Snapshot {
	return Snapshot{Metadata: Metadata{Version: 1, Source: "iplist", URL: "https://example.org/", UpdatedAt: time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)}, Payload: []byte("{}")}
}

func putRaw(t *testing.T, dir string, m Metadata, payload string) {
	t.Helper()
	b, err := json.Marshal(m)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "iplist.snapshot"), append(append(b, '\n'), payload...), 0600))
}

func TestWriteReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := testSnapshot()
	require.NoError(t, Write(context.Background(), dir, want))
	got, err := Read(dir, "iplist")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestReadRejectsUnknownVersion(t *testing.T) {
	dir := t.TempDir()
	m := testSnapshot().Metadata
	m.Version = 2
	putRaw(t, dir, m, "{}")
	_, err := Read(dir, "iplist")
	require.Error(t, err)
}

func TestReadMissingFile(t *testing.T) {
	_, err := Read(t.TempDir(), "iplist")
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestReadRejectsSourceMismatch(t *testing.T) {
	dir := t.TempDir()
	m := testSnapshot().Metadata
	m.Source = "v2fly"
	putRaw(t, dir, m, "{}")
	_, err := Read(dir, "iplist")
	require.Error(t, err)
}

func TestReadRejectsInvalidSource(t *testing.T) {
	_, err := Read(t.TempDir(), "../outside")
	require.Error(t, err)
}

func TestWriteRejectsInvalidURL(t *testing.T) {
	s := testSnapshot()
	s.Metadata.URL = ":invalid"
	require.Error(t, Write(context.Background(), t.TempDir(), s))
}

func TestWriteRejectsMissingTime(t *testing.T) {
	s := testSnapshot()
	s.Metadata.UpdatedAt = time.Time{}
	require.Error(t, Write(context.Background(), t.TempDir(), s))
}

func TestReadRejectsTruncatedHeader(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "iplist.snapshot"), []byte(`{"version":1}`), 0600))
	_, err := Read(dir, "iplist")
	require.Error(t, err)
}

func TestReadRejectsOversizedHeader(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "iplist.snapshot"), []byte(strings.Repeat(" ", 16385)+"\n{}"), 0600))
	_, err := Read(dir, "iplist")
	require.Error(t, err)
}

func TestWriteCancellationPreservesSnapshot(t *testing.T) {
	dir := t.TempDir()
	want := testSnapshot()
	require.NoError(t, Write(context.Background(), dir, want))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := want
	s.Payload = []byte(`{"changed":true}`)
	require.ErrorIs(t, Write(ctx, dir, s), context.Canceled)
	got, err := Read(dir, "iplist")
	require.NoError(t, err)
	assert.Equal(t, want, got)
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, files, 1)
}

func TestWriteRejectsFileAsDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(path, []byte("untouched"), 0600))
	require.Error(t, Write(context.Background(), path, testSnapshot()))
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "untouched", string(b))
}

func TestWriteReadersSeeCompleteSnapshots(t *testing.T) {
	dir := t.TempDir()
	a := testSnapshot()
	b := testSnapshot()
	b.Payload = []byte(strings.Repeat("b", 4096))
	b.Metadata.URL = "https://other.example/"
	require.NoError(t, Write(context.Background(), dir, a))
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			got, err := Read(dir, "iplist")
			if assert.NoError(t, err) {
				assert.True(t, string(got.Payload) == string(a.Payload) && got.Metadata == a.Metadata || string(got.Payload) == string(b.Payload) && got.Metadata == b.Metadata)
			}
		}
	}()
	for i := 0; i < 20; i++ {
		require.NoError(t, Write(context.Background(), dir, b))
		require.NoError(t, Write(context.Background(), dir, a))
	}
	wg.Wait()
}

func TestWriteRejectsOversizedPayload(t *testing.T) {
	s := testSnapshot()
	s.Metadata.Source = "v2fly"
	s.Payload = make([]byte, (32<<20)+1)
	require.Error(t, Write(context.Background(), t.TempDir(), s))
}

func TestReadRejectsOversizedPayload(t *testing.T) {
	dir := t.TempDir()
	putRaw(t, dir, testSnapshot().Metadata, strings.Repeat("x", (64<<20)+1))
	_, err := Read(dir, "iplist")
	require.Error(t, err)
}
