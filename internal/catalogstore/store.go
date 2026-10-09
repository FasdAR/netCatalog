// Package catalogstore stores independently replaceable, versioned source snapshots.
package catalogstore

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const maxHeader = 16 << 10

// Metadata identifies a source snapshot and its successful download time.
type Metadata struct {
	Version   int       `json:"version"`
	Source    string    `json:"source"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Snapshot keeps metadata and the complete upstream JSON or YAML payload together.
type Snapshot struct {
	Metadata Metadata
	Payload  []byte
}

func sourceLimit(source string) (int64, error) {
	switch source {
	case "iplist":
		return 64 << 20, nil
	case "v2fly":
		return 32 << 20, nil
	default:
		return 0, fmt.Errorf("unsupported snapshot source %q", source)
	}
}

func validate(m Metadata, source string) error {
	if m.Version != 1 {
		return fmt.Errorf("unsupported snapshot version %d", m.Version)
	}
	if m.Source != source {
		return fmt.Errorf("snapshot source %q does not match %q", m.Source, source)
	}
	u, err := url.Parse(m.URL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Fragment != "" {
		return fmt.Errorf("invalid snapshot URL %q", m.URL)
	}
	_, offset := m.UpdatedAt.Zone()
	if m.UpdatedAt.IsZero() || offset != 0 {
		return fmt.Errorf("snapshot timestamp must be a nonzero UTC time")
	}
	return nil
}

// Read loads a bounded snapshot and validates its version and metadata.
// Payload semantics are validated by the corresponding source provider.
func Read(dir, source string) (snapshot Snapshot, err error) {
	limit, err := sourceLimit(source)
	if err != nil {
		return snapshot, err
	}
	f, err := os.Open(filepath.Join(dir, source+".snapshot"))
	if err != nil {
		return snapshot, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	info, err := f.Stat()
	if err != nil {
		return snapshot, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit+maxHeader {
		return snapshot, fmt.Errorf("invalid or oversized %s snapshot", source)
	}
	r := bufio.NewReaderSize(f, maxHeader)
	header, err := r.ReadSlice('\n')
	if err != nil {
		return snapshot, fmt.Errorf("invalid snapshot header: %w", err)
	}
	if err := json.Unmarshal(header, &snapshot.Metadata); err != nil {
		return snapshot, fmt.Errorf("invalid snapshot metadata: %w", err)
	}
	if err := validate(snapshot.Metadata, source); err != nil {
		return snapshot, err
	}
	snapshot.Payload, err = io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return snapshot, err
	}
	if len(snapshot.Payload) == 0 || int64(len(snapshot.Payload)) > limit {
		return snapshot, fmt.Errorf("empty or oversized %s snapshot payload", source)
	}
	return snapshot, nil
}

// Write publishes a snapshot using a temporary file and an atomic replacement.
// Failure before publication leaves the old snapshot unchanged. Callers must
// validate the payload's source schema before calling Write.
// Non-Unix publication is rejected because os.Rename does not guarantee atomicity.
func Write(ctx context.Context, dir string, snapshot Snapshot) (err error) {
	limit, err := sourceLimit(snapshot.Metadata.Source)
	if err != nil {
		return err
	}
	if err := validate(snapshot.Metadata, snapshot.Metadata.Source); err != nil {
		return err
	}
	if len(snapshot.Payload) == 0 || int64(len(snapshot.Payload)) > limit {
		return fmt.Errorf("empty or oversized snapshot payload")
	}
	header, err := json.Marshal(snapshot.Metadata)
	if err != nil {
		return err
	}
	if len(header)+1 > maxHeader {
		return fmt.Errorf("snapshot metadata exceeds 16 KiB")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+snapshot.Metadata.Source+"-*.tmp")
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			err = errors.Join(err, f.Close())
		}
		if removeErr := os.Remove(f.Name()); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			err = errors.Join(err, removeErr)
		}
	}()
	if _, err := f.Write(append(header, '\n')); err != nil {
		return err
	}
	if _, err := f.Write(snapshot.Payload); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	err = f.Close()
	closed = true
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return PublishSnapshot(runtime.GOOS, f.Name(), filepath.Join(dir, snapshot.Metadata.Source+".snapshot"))
}

// PublishSnapshot atomically replaces a snapshot on supported Unix platforms.
// Unsupported platforms leave both paths unchanged and return an error.
func PublishSnapshot(goos, oldPath, newPath string) error {
	switch goos {
	case "aix", "android", "darwin", "dragonfly", "freebsd", "illumos", "ios", "linux", "netbsd", "openbsd", "solaris":
		return os.Rename(oldPath, newPath)
	default:
		return fmt.Errorf("atomic snapshot publication is not supported on %s; previous snapshot unchanged", goos)
	}
}
