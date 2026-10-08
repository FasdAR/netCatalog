package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"netCatalog/internal/catalogstore"
	"netCatalog/internal/providers/iplist"
	"netCatalog/internal/providers/v2fly"
	"netCatalog/internal/resolver"
)

type updateSource struct {
	id       string
	url      string
	download func(context.Context, string, *http.Client) ([]byte, error)
	parse    func([]byte) (resolver.Provider, error)
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Download and atomically replace local source catalogs",
	Long: "Download full iplist JSON and compiled v2fly YAML into --data-dir.\n" +
		"Use --source iplist or --source v2fly to update only one source.\n" +
		"A failed update keeps the previous snapshot; other successful updates are retained.\n" +
		"Exit code 1 means at least one selected source failed to update.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		o := rootOptions
		if o.timeout <= 0 {
			return fmt.Errorf("timeout must be positive")
		}
		if o.source == "v2fly" && o.v2flyURL == "" {
			return fmt.Errorf("--source v2fly requires a non-empty --v2fly-url")
		}
		client := &http.Client{Timeout: o.timeout}
		sources := []updateSource{}
		if o.source != "v2fly" {
			if _, err := iplist.New(o.iplistURL, client); err != nil {
				return err
			}
			sources = append(sources, updateSource{id: "iplist", url: o.iplistURL, download: iplist.Download, parse: func(b []byte) (resolver.Provider, error) { return iplist.NewSnapshot(b) }})
		}
		if o.source != "iplist" && o.v2flyURL != "" {
			if _, err := v2fly.New(o.v2flyURL, client); err != nil {
				return err
			}
			sources = append(sources, updateSource{id: "v2fly", url: o.v2flyURL, download: v2fly.Download, parse: func(b []byte) (resolver.Provider, error) { return v2fly.NewSnapshot(b) }})
		}
		dir, err := o.catalogDir()
		if err != nil {
			return runtimeError(err)
		}
		var failures []error
		for _, source := range sources {
			count, err := saveSource(cmd.Context(), dir, source, client)
			if err != nil {
				failures = append(failures, fmt.Errorf("%s: %w", source.id, err))
				continue
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: updated %d services in %s\n", source.id, count, filepath.Join(dir, source.id+".snapshot")); err != nil {
				failures = append(failures, err)
			}
		}
		if err := errors.Join(failures...); err != nil {
			return runtimeError(err)
		}
		return nil
	},
}

func saveSource(ctx context.Context, dir string, source updateSource, client *http.Client) (int, error) {
	payload, err := source.download(ctx, source.url, client)
	if err != nil {
		return 0, err
	}
	p, err := source.parse(payload)
	if err != nil {
		return 0, err
	}
	services, catalogueErr := p.ListServices(ctx)
	// Invalid portal identities can be skipped, but cancellation is not a usable export.
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if catalogueErr != nil && len(services) == 0 {
		return 0, catalogueErr
	}
	snapshot := catalogstore.Snapshot{
		Metadata: catalogstore.Metadata{Version: 1, Source: source.id, URL: source.url, UpdatedAt: time.Now().UTC()},
		Payload:  payload,
	}
	if err := catalogstore.Write(ctx, dir, snapshot); err != nil {
		return 0, err
	}
	return len(services), nil
}

func init() { rootCmd.AddCommand(updateCmd) }
