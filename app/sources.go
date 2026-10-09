package app

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newSourcesCommand() *cobra.Command {
	sourcesCmd := &cobra.Command{
		Use:   "sources",
		Short: "Show supported data sources",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("expected sources list")
		},
	}

	sourcesCmd.AddCommand(newSourcesListCommand())
	return sourcesCmd
}
