package app

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"netCatalog/internal/resolver"
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
	for _, name := range shortNames(catalogue.Search(term)) {
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), name); err != nil {
			return runtimeError(err)
		}
	}
	return nil
}

// shortNames returns sorted unique service names: the first label of a portal
// domain (telegram.org -> telegram) or the list name, so a portal and a list
// of the same service are shown once.
func shortNames(services []resolver.Service) []string {
	seen := make(map[string]struct{}, len(services))
	names := make([]string, 0, len(services))
	for _, service := range services {
		name, _, _ := strings.Cut(service.Identifier(), ".")
		if _, ok := seen[name]; ok || name == "" {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
