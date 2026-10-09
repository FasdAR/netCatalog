package cmd

import (
	"context"
	"io"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
)

// The production tree is executed once per process. These tests deliberately
// reuse it, so flag values, Changed markers and execution resources are reset.
func resetCommand(ctx context.Context, t *testing.T, cmd *cobra.Command) {
	t.Helper()
	resetFlag := func(flag *pflag.Flag) {
		require.NoError(t, flag.Value.Set(flag.DefValue))
		flag.Changed = false
	}
	cmd.Flags().VisitAll(resetFlag)
	cmd.PersistentFlags().VisitAll(resetFlag)
	cmd.SetArgs([]string{})
	cmd.SetOut(nil)
	cmd.SetErr(nil)
	// Set the invocation context explicitly throughout the reused command tree.
	cmd.SetContext(ctx)
	for _, child := range cmd.Commands() {
		resetCommand(ctx, t, child)
	}
}

func runForTest(ctx context.Context, t *testing.T, args []string, stdout, stderr io.Writer, defaultV2flyURL string) int {
	t.Helper()
	resetCommand(ctx, t, rootCmd)
	t.Cleanup(func() { resetCommand(context.Background(), t, rootCmd) })
	rootOptions.dataDir = t.TempDir()
	rootOptions.v2flyURL = defaultV2flyURL
	return run(ctx, args, stdout, stderr)
}
