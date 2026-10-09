package app

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

const orange = "\x1b[38;5;208m"
const resetColor = "\x1b[0m"

type warningPrinter struct {
	writer  io.Writer
	enabled bool
	color   bool
}

func (p warningPrinter) print(warnings []string) {
	PrintWarnings(p.writer, p.enabled, p.color, warnings)
}

// PrintWarnings writes enabled warnings, optionally using terminal colors.
// It stops after the first write failure.
func PrintWarnings(writer io.Writer, enabled, color bool, warnings []string) {
	if !enabled {
		return
	}
	for _, warning := range warnings {
		if color {
			if _, err := fmt.Fprintf(writer, "%swarning: %s%s\n", orange, warning, resetColor); err != nil {
				return
			}
		} else {
			if _, err := fmt.Fprintln(writer, "warning:", warning); err != nil {
				return
			}
		}
	}
}

// SupportsColor reports whether a stream is a terminal with colors enabled.
func SupportsColor(writer io.Writer) bool {
	if _, disabled := os.LookupEnv("NO_COLOR"); disabled || os.Getenv("TERM") == "dumb" {
		return false
	}
	file, ok := writer.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}
