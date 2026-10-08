package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newServicesCommand(o *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "services",
		Short: "Discover services in the available sources",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("expected services list or services search TERM")
		},
	}
	list := &cobra.Command{Use: "list", Short: "List service identifiers", Args: cobra.NoArgs}
	search := &cobra.Command{Use: "search TERM", Short: "Search service identifiers by substring", Args: cobra.ExactArgs(1)}
	list.RunE = func(cmd *cobra.Command, _ []string) error { return listServices(cmd, o, "") }
	search.RunE = func(cmd *cobra.Command, args []string) error { return listServices(cmd, o, args[0]) }
	cmd.AddCommand(list, search)
	return cmd
}

func listServices(cmd *cobra.Command, o *options, term string) error {
	engine, _, err := o.engine(true)
	if err != nil {
		return err
	}
	catalogue, err := loadCatalogue(cmd.Context(), engine, o.warnings(cmd.ErrOrStderr()))
	if err != nil {
		return err
	}
	for _, service := range catalogue.Search(term) {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), service.Identifier()); err != nil {
			return runtimeError(err)
		}
	}
	return nil
}
