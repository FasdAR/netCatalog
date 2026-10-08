package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newSourcesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sources",
		Short: "Show supported data sources",
		Args:  cobra.NoArgs,
		RunE:  func(_ *cobra.Command, _ []string) error { return fmt.Errorf("expected sources list") },
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List sources and supported data types",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "iplist\tdomains,ips,cidrs\nv2fly\tdomains")
			if err != nil {
				return runtimeError(err)
			}
			return nil
		},
	})
	return cmd
}
