// Package cli implements netCatalog commands and separates process management
// from argument parsing, data retrieval, and output.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"netCatalog/internal/providers/v2fly"
)

// Run executes a command using args and writes data to stdout and diagnostics to
// stderr. It returns 0 for successful, partial, or empty results, 1 for runtime
// failures, and 2 for invalid arguments or unknown or ambiguous services.
// Run does not terminate the process. Each call creates fresh command state.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return run(ctx, args, stdout, stderr, v2fly.DefaultURL)
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, defaultV2flyURL string) int {
	out, diagnostics := &checkedWriter{writer: stdout}, &checkedWriter{writer: stderr}
	root := newRootCommand(defaultV2flyURL, stderr)
	root.SetArgs(args)
	root.SetOut(out)
	root.SetErr(diagnostics)
	err := root.ExecuteContext(ctx)
	// Cobra's help renderer does not return write errors, so check the streams
	// as well as ExecuteContext. Redirected output must not report success if lost.
	if out.err != nil {
		if _, err := fmt.Fprintln(diagnostics, "error:", out.err); err != nil {
			return 1
		}
		return 1
	}
	if diagnostics.err != nil {
		return 1
	}
	if err == nil {
		return 0
	}
	code := 2
	var failure *commandError
	if errors.As(err, &failure) {
		code = failure.code
	}
	if _, err := fmt.Fprintln(diagnostics, "error:", err); err != nil {
		return 1
	}
	return code
}

type commandError struct {
	code int
	err  error
}

func (e *commandError) Error() string { return e.err.Error() }
func (e *commandError) Unwrap() error { return e.err }

func runtimeError(err error) error { return &commandError{code: 1, err: err} }

type checkedWriter struct {
	writer io.Writer
	err    error
}

func (w *checkedWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	n, err := w.writer.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	w.err = err
	return n, err
}
