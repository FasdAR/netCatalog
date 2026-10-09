package app

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newSourcesListCommand() *cobra.Command {
	return &cobra.Command{
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
	}

}
