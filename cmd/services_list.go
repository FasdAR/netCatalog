package cmd

import "github.com/spf13/cobra"

var servicesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List service identifiers from local catalogs",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return listServices(cmd, rootOptions, "")
	},
}

func init() {
	servicesCmd.AddCommand(servicesListCmd)
}
