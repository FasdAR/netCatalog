package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"netCatalog/internal/resolver"
)

func newGetCommand(o *options) *cobra.Command {
	var kind, family string
	var site bool
	cmd := &cobra.Command{
		Use:   "get SERVICE",
		Short: "Get domains, IP addresses, or CIDR networks for a service",
		Long: "Get records for SERVICE, one per line. Short names (discord) combine\n" +
			"portals with the same first label (discord.*) and matching named lists.\n" +
			"Exact domains (discord.com) select only that portal. Failed requests and invalid records are skipped.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			term := args[0]
			if strings.TrimSpace(term) == "" {
				return fmt.Errorf("SERVICE must not be empty")
			}
			if kind != "domains" && kind != "ips" && kind != "cidrs" {
				return fmt.Errorf("--type must be domains, ips or cidrs")
			}
			if family != "ipv4" && family != "ipv6" && family != "all" {
				return fmt.Errorf("--family must be ipv4, ipv6 or all")
			}
			if kind == "domains" && cmd.Flags().Changed("family") {
				return fmt.Errorf("--family does not apply to domains")
			}
			engine, p, err := o.engine(!site && kind == "domains")
			if err != nil {
				return err
			}
			warnings := o.warnings(cmd.ErrOrStderr())
			var selection resolver.Selection
			if site {
				domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(term), "."))
				selection = resolver.Selection{Name: domain, Services: []resolver.Service{{Domain: domain, Bindings: []resolver.Binding{{Source: p.ID(), Key: domain}}}}}
			} else {
				catalogue, err := loadCatalogue(cmd.Context(), engine, warnings)
				if err != nil {
					return err
				}
				selection, err = catalogue.Resolve(term)
				if err != nil {
					return err
				}
			}
			result := engine.Fetch(cmd.Context(), selection, resolver.Query{Kind: resolver.DataKind(kind), Family: resolver.Family(family)})
			warnings.print(result.Warnings)
			for _, entry := range result.Entries {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), entry.Value); err != nil {
					return runtimeError(err)
				}
			}
			if result.Succeeded == 0 {
				return runtimeError(fmt.Errorf("all data requests failed; use --info for details"))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&kind, "type", "", "domains, ips or cidrs (required)")
	cmd.Flags().StringVar(&family, "family", "ipv4", "ipv4, ipv6 or all (not for domains)")
	cmd.Flags().BoolVar(&site, "site", false, "query an exact iplist portal without catalogue discovery")
	return cmd
}
