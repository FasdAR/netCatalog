package app

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newServicesCommand(o *options) *cobra.Command {
	servicesCmd := &cobra.Command{
		Use:   "services",
		Short: "Discover services in the available sources",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fmt.Errorf("expected services list or services search TERM")
		},
	}

	servicesCmd.AddCommand(newServicesListCommand(o), newServicesSearchCommand(o))
	return servicesCmd
}

func listServices(cmd *cobra.Command, o *options, term string) error {
	engine, err := o.engine(true)
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
