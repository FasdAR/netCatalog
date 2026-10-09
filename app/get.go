package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"netCatalog/internal/resolver"
)

func newGetCommand(rootOptions *options) *cobra.Command {
	var kind, family, outputDir string
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
			if result.Succeeded == 0 {
				return runtimeError(fmt.Errorf("all local data reads failed: %v; run netCatalog update", result.Warnings))
			}
			var output strings.Builder
			for _, entry := range result.Entries {
				output.WriteString(entry.Value + "\n")
			}
			if outputDir != "" {
				return writeServiceFile(outputDir, selection.Name, kind, output.String())
			}
			if _, err := fmt.Fprint(cmd.OutOrStdout(), output.String()); err != nil {
				return runtimeError(err)
			}
			return nil
		},
	}

	getCmd.Flags().StringVar(&kind, "type", "", "domain, ip or cidr (required)")
	getCmd.Flags().StringVar(&family, "family", "ipv4", "ipv4, ipv6 or all (not for domain)")
	getCmd.Flags().StringVar(&outputDir, "output-dir", "", "write results to DIR/SERVICE/TYPE.md instead of stdout")
	return getCmd
}

// writeServiceFile saves records to dir/service/kind.md, creating directories as needed.
func writeServiceFile(dir, service, kind, content string) error {
	if service == "" || service == "." || service == ".." || filepath.Base(service) != service {
		return runtimeError(fmt.Errorf("service name %q cannot be used as a directory name", service))
	}
	serviceDir := filepath.Join(dir, service)
	if err := os.MkdirAll(serviceDir, 0o755); err != nil {
		return runtimeError(err)
	}
	if err := os.WriteFile(filepath.Join(serviceDir, kind+".md"), []byte(content), 0o644); err != nil {
		return runtimeError(err)
	}
	return nil
}
