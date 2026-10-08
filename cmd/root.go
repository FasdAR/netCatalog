// Package cmd implements netCatalog's Cobra commands and output contracts.
package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"netCatalog/internal/catalogstore"
	"netCatalog/internal/providers/iplist"
	"netCatalog/internal/providers/v2fly"
)

type options struct {
	dataDir   string
	source    string
	iplistURL string
	v2flyURL  string
	timeout   time.Duration
	info      bool
	color     bool
}

func (o *options) catalogDir() (string, error) {
	if o.dataDir != "" {
		return o.dataDir, nil
	}
	return catalogstore.DefaultDir()
}

var rootOptions = &options{}

var rootCmd = &cobra.Command{
	Use:   "netCatalog",
	Short: "Get service domains, IP addresses, and CIDR networks",
	Long: "netCatalog searches local service catalogs without network access.\n" +
		"Run netCatalog update first to download the latest iplist and v2fly snapshots.\n" +
		"Records are printed one per line; warnings require --info.\n" +
		"Exit codes: 0 success (including partial/empty results), 1 runtime failure, 2 invalid input.",
	SilenceUsage:  true,
	SilenceErrors: true,
	Args:          cobra.NoArgs,
	RunE:          func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
		if rootOptions.source != "" && rootOptions.source != "iplist" && rootOptions.source != "v2fly" {
			return fmt.Errorf("--source must be iplist or v2fly")
		}
		return nil
	},
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	flags := rootCmd.PersistentFlags()
	flags.StringVar(&rootOptions.dataDir, "data-dir", "", "local catalog directory (default: user data directory)")
	flags.StringVar(&rootOptions.source, "source", "", "use only iplist or v2fly for update, get and services (default: all applicable sources)")
	flags.StringVar(&rootOptions.iplistURL, "iplist-url", iplist.DefaultURL, "iplist HTTP endpoint (update only)")
	flags.StringVar(&rootOptions.v2flyURL, "v2fly-url", v2fly.DefaultURL, "compiled v2fly YAML URL (update only; empty skips v2fly during update)")
	flags.DurationVar(&rootOptions.timeout, "timeout", 30*time.Second, "timeout per HTTP request (update only)")
	flags.BoolVar(&rootOptions.info, "info", false, "show warnings on stderr (orange in a terminal)")
}
