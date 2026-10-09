package app

import "github.com/spf13/cobra"

func newServicesListCommand(rootOptions *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List service identifiers from local catalogs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return listServices(cmd, rootOptions, "")
		},
	}

}
