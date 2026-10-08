package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"netCatalog/internal/providers/iplist"
	"netCatalog/internal/providers/v2fly"
	"netCatalog/internal/resolver"
)

type options struct {
	url      string
	v2flyURL string
	timeout  time.Duration
	info     bool
	color    bool
}

func newRootCommand(defaultV2flyURL string, stderr io.Writer) *cobra.Command {
	o := &options{color: supportsColor(stderr)}
	root := &cobra.Command{
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
			if o.timeout <= 0 {
				return fmt.Errorf("timeout must be positive")
			}
			return nil
		},
	}
	root.CompletionOptions.DisableDefaultCmd = true
	flags := root.PersistentFlags()
	flags.StringVar(&o.url, "url", iplist.DefaultURL, "iplist HTTP endpoint")
	flags.StringVar(&o.v2flyURL, "v2fly-url", defaultV2flyURL, "compiled v2fly YAML release URL (empty disables the source)")
	flags.DurationVar(&o.timeout, "timeout", 30*time.Second, "timeout per HTTP request")
	flags.BoolVar(&o.info, "info", false, "show warnings on stderr (orange in a terminal)")
	root.AddCommand(newGetCommand(o), newServicesCommand(o), newSourcesCommand())
	return root
}

func (o *options) engine(includeV2fly bool) (resolver.Resolver, *iplist.Provider, error) {
	client := &http.Client{Timeout: o.timeout}
	p, err := iplist.New(o.url, client)
	if err != nil {
		return resolver.Resolver{}, nil, err
	}
	engine := resolver.Resolver{Providers: []resolver.Provider{p}}
	if includeV2fly && o.v2flyURL != "" {
		provider, err := v2fly.New(o.v2flyURL, client)
		if err != nil {
			return resolver.Resolver{}, nil, err
		}
		engine.Providers = append(engine.Providers, provider)
	}
	return engine, p, nil
}

func (o *options) warnings(writer io.Writer) warningPrinter {
	return warningPrinter{writer: writer, enabled: o.info, color: o.color}
}

func loadCatalogue(ctx context.Context, engine resolver.Resolver, warnings warningPrinter) (resolver.Catalogue, error) {
	catalogue := engine.ListServices(ctx)
	warnings.print(catalogue.Warnings)
	if catalogue.Succeeded == 0 && len(catalogue.Services) == 0 {
		return catalogue, runtimeError(fmt.Errorf("unable to load the service catalogue; use --info for details"))
	}
	return catalogue, nil
}
