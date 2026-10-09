package app

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"netCatalog/internal/resolver"
)

func newGetCommand(rootOptions *options) *cobra.Command {
	var kind, family string
	getCmd := &cobra.Command{
		Use:   "get SERVICE",
		Short: "Get domains, IP addresses, or CIDR networks for a service",
		Long: "Get records from local snapshots for SERVICE, one per line; run netCatalog update first.\n" +
			"Short names (discord) combine\n" +
			"portals with the same first label (discord.*) and matching named lists.\n" +
			"Exact domains (discord.com) select only that portal. Invalid records are skipped; no HTTP requests are made.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			term := args[0]
			if strings.TrimSpace(term) == "" {
				return fmt.Errorf("SERVICE must not be empty")
			}
			if kind != "domain" && kind != "ip" && kind != "cidr" {
				return fmt.Errorf("--type must be domain, ip or cidr")
			}
			if family != "ipv4" && family != "ipv6" && family != "all" {
				return fmt.Errorf("--family must be ipv4, ipv6 or all")
			}
			if kind == "domain" && cmd.Flags().Changed("family") {
				return fmt.Errorf("--family does not apply to domain type")
			}
			if rootOptions.source == "v2fly" && kind != "domain" {
				return fmt.Errorf("--source v2fly supports only --type domain")
			}
			engine, err := rootOptions.engine(kind == "domain")
			if err != nil {
				return err
			}
			warnings := rootOptions.warnings(cmd.ErrOrStderr())
			catalogue, err := loadCatalogue(cmd.Context(), engine, warnings)
			if err != nil {
				return err
			}
			selection, err := catalogue.Resolve(term)
			if err != nil {
				return err
			}
			result := engine.Fetch(cmd.Context(), selection, resolver.Query{Kind: resolver.DataKind(kind), Family: resolver.Family(family)})
			warnings.print(result.Warnings)
			for _, entry := range result.Entries {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), entry.Value); err != nil {
					return runtimeError(err)
				}
			}
			if result.Succeeded == 0 {
				return runtimeError(fmt.Errorf("all local data reads failed: %v; run netCatalog update", result.Warnings))
			}
			return nil
		},
	}

	getCmd.Flags().StringVar(&kind, "type", "", "domain, ip or cidr (required)")
	getCmd.Flags().StringVar(&family, "family", "ipv4", "ipv4, ipv6 or all (not for domain)")
	return getCmd
}
