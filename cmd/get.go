package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"netCatalog/internal/resolver"
)

var kind, family string
var site bool

var getCmd = &cobra.Command{
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
		if rootOptions.source == "v2fly" {
			if site {
				return fmt.Errorf("--site requires iplist; it cannot be used with --source v2fly")
			}
			if kind != "domains" {
				return fmt.Errorf("--source v2fly supports only --type domains")
			}
		}
		engine, p, err := rootOptions.engine(!site && kind == "domains")
		if err != nil {
			return err
		}
		warnings := rootOptions.warnings(cmd.ErrOrStderr())
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

func init() {
	rootCmd.AddCommand(getCmd)
	getCmd.Flags().StringVar(&kind, "type", "", "domains, ips or cidrs (required)")
	getCmd.Flags().StringVar(&family, "family", "ipv4", "ipv4, ipv6 or all (not for domains)")
	getCmd.Flags().BoolVar(&site, "site", false, "query an exact iplist portal without catalogue discovery")
}
