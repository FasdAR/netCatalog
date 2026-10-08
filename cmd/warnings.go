package cmd

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
	if !p.enabled {
		return
	}
	for _, warning := range warnings {
		if p.color {
			if _, err := fmt.Fprintf(p.writer, "%swarning: %s%s\n", orange, warning, resetColor); err != nil {
				return
			}
		} else {
			if _, err := fmt.Fprintln(p.writer, "warning:", warning); err != nil {
				return
			}
		}
	}
}

func supportsColor(writer io.Writer) bool {
	if _, disabled := os.LookupEnv("NO_COLOR"); disabled || os.Getenv("TERM") == "dumb" {
		return false
	}
	file, ok := writer.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}
