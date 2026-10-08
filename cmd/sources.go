package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var sourcesCmd = &cobra.Command{
	Use:   "sources",
	Short: "Show supported data sources",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		return fmt.Errorf("expected sources list")
	},
}

func init() {
	rootCmd.AddCommand(sourcesCmd)
}
