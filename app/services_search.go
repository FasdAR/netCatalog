package app

import "github.com/spf13/cobra"

func newServicesSearchCommand(rootOptions *options) *cobra.Command {
	return &cobra.Command{
		Use:   "search TERM",
		Short: "Search local service identifiers by substring",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return listServices(cmd, rootOptions, args[0])
		},
	}

}
