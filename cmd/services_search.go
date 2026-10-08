package cmd

import "github.com/spf13/cobra"

var servicesSearchCmd = &cobra.Command{
	Use:   "search TERM",
	Short: "Search service identifiers by substring",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return listServices(cmd, rootOptions, args[0])
	},
}

func init() {
	servicesCmd.AddCommand(servicesSearchCmd)
}
