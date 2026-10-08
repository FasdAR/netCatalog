package cmd

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWarningPrinterHidesDisabledWarnings(t *testing.T) {
	var out bytes.Buffer
	warningPrinter{writer: &out}.print([]string{"skipped record"})
	assert.Empty(t, out.String())
}

func TestWarningPrinterHidesDisabledColoredWarnings(t *testing.T) {
	var out bytes.Buffer
	warningPrinter{writer: &out, color: true}.print([]string{"skipped record"})
	assert.Empty(t, out.String())
}

func TestWarningPrinterPrintsPlainWarnings(t *testing.T) {
	var out bytes.Buffer
	warningPrinter{writer: &out, enabled: true}.print([]string{"skipped record"})
	assert.Equal(t, "warning: skipped record\n", out.String())
}

func TestWarningPrinterPrintsColoredWarnings(t *testing.T) {
	var out bytes.Buffer
	warningPrinter{writer: &out, enabled: true, color: true}.print([]string{"skipped record"})
	assert.Equal(t, "\x1b[38;5;208mwarning: skipped record\x1b[0m\n", out.String())
}

type countedFailureWriter struct{ calls int }

func (w *countedFailureWriter) Write([]byte) (int, error) {
	w.calls++
	return 0, errors.New("output unavailable")
}

func TestWarningPrinterStopsAfterPlainWriteFailure(t *testing.T) {
	writer := &countedFailureWriter{}
	warningPrinter{writer: writer, enabled: true}.print([]string{"first", "second"})
	assert.Equal(t, 1, writer.calls)
}

func TestWarningPrinterStopsAfterColoredWriteFailure(t *testing.T) {
	writer := &countedFailureWriter{}
	warningPrinter{writer: writer, enabled: true, color: true}.print([]string{"first", "second"})
	assert.Equal(t, 1, writer.calls)
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	value, exists := os.LookupEnv(key)
	require.NoError(t, os.Unsetenv(key))
	t.Cleanup(func() {
		if exists {
			require.NoError(t, os.Setenv(key, value))
		} else {
			require.NoError(t, os.Unsetenv(key))
		}
	})
}

func plainTerminalEnv(t *testing.T) {
	t.Helper()
	unsetEnv(t, "NO_COLOR")
	t.Setenv("TERM", "xterm-256color")
}

func TestSupportsColorRejectsBuffer(t *testing.T) {
	plainTerminalEnv(t)
	var buffer bytes.Buffer
	assert.False(t, supportsColor(&buffer))
}

func TestSupportsColorRejectsRegularFile(t *testing.T) {
	plainTerminalEnv(t)
	file, err := os.CreateTemp(t.TempDir(), "stderr")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, file.Close()) })
	assert.False(t, supportsColor(file))
}

func TestSupportsColorRejectsNullDevice(t *testing.T) {
	plainTerminalEnv(t)
	device, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, device.Close()) })
	assert.False(t, supportsColor(device))
}

func TestSupportsColorRejectsPipe(t *testing.T) {
	plainTerminalEnv(t)
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reader.Close()); require.NoError(t, writer.Close()) })
	assert.False(t, supportsColor(writer))
}

func TestSupportsColorHonorsEmptyNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	assert.False(t, supportsColor(os.Stderr))
}

func TestSupportsColorHonorsDumbTerminal(t *testing.T) {
	unsetEnv(t, "NO_COLOR")
	t.Setenv("TERM", "dumb")
	assert.False(t, supportsColor(os.Stderr))
}
