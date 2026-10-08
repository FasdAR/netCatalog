// Package cmd implements netCatalog's Cobra commands and output contracts.
package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"netCatalog/internal/providers/iplist"
	"netCatalog/internal/providers/v2fly"
)

type options struct {
	iplistURL string
	v2flyURL  string
	timeout   time.Duration
	info      bool
	color     bool
}

var rootOptions = &options{}

var rootCmd = &cobra.Command{
	Use:   "netCatalog",
	Short: "Get service domains, IP addresses, and CIDR networks",
	Long: "netCatalog retrieves public service lists over HTTP without a persistent cache.\n" +
		"Records are printed one per line; warnings require --info.\n" +
		"Exit codes: 0 success (including partial/empty results), 1 runtime failure, 2 invalid input.",
	SilenceUsage:  true,
	SilenceErrors: true,
	Args:          cobra.NoArgs,
	RunE:          func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
		if rootOptions.timeout <= 0 {
			return fmt.Errorf("timeout must be positive")
		}
		return nil
	},
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	flags := rootCmd.PersistentFlags()
	flags.StringVar(&rootOptions.iplistURL, "iplist-url", iplist.DefaultURL, "iplist HTTP endpoint")
	flags.StringVar(&rootOptions.v2flyURL, "v2fly-url", v2fly.DefaultURL, "compiled v2fly YAML release URL (empty disables the source)")
	flags.DurationVar(&rootOptions.timeout, "timeout", 30*time.Second, "timeout per HTTP request")
	flags.BoolVar(&rootOptions.info, "info", false, "show warnings on stderr (orange in a terminal)")
}
