package cli

import (
	"bytes"
	"os"
	"testing"
)

func TestWarningPrinter(t *testing.T) {
	for _, tt := range []struct {
		enabled, color bool
		want           string
	}{
		{false, false, ""}, {false, true, ""},
		{true, false, "warning: skipped record\n"},
		{true, true, orange + "warning: skipped record" + resetColor + "\n"},
	} {
		var out bytes.Buffer
		warningPrinter{writer: &out, enabled: tt.enabled, color: tt.color}.print([]string{"skipped record"})
		if out.String() != tt.want {
			t.Fatalf("got %q, want %q", out.String(), tt.want)
		}
	}
}

func TestColorDisabledForRedirectedOutput(t *testing.T) {
	var buffer bytes.Buffer
	if supportsColor(&buffer) {
		t.Fatal("buffer must not use color")
	}
	file, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if supportsColor(file) {
		t.Fatal("regular file must not use color")
	}
	if isTerminal(file.Fd()) {
		t.Fatal("regular file is not a terminal")
	}
	device, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer device.Close()
	if supportsColor(device) {
		t.Fatal("character device is not necessarily a terminal")
	}
}

func TestColorDisabledByEnvironment(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	if supportsColor(os.Stderr) {
		t.Fatal("NO_COLOR presence must disable color")
	}
	t.Setenv("TERM", "dumb")
	if supportsColor(os.Stderr) {
		t.Fatal("dumb terminal must not use color")
	}
}
