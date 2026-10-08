package cmd

import (
	"context"
	"fmt"
	"io"

	"netCatalog/internal/catalogstore"
	"netCatalog/internal/providers/iplist"
	"netCatalog/internal/providers/v2fly"
	"netCatalog/internal/resolver"
)

func (o *options) engine(includeV2fly bool) (resolver.Resolver, *iplist.SnapshotProvider, error) {
	dir, err := o.catalogDir()
	if err != nil {
		return resolver.Resolver{}, nil, runtimeError(err)
	}
	var p *iplist.SnapshotProvider
	var engine resolver.Resolver
	if o.source != "v2fly" {
		snapshot, loadErr := catalogstore.Read(dir, "iplist")
		if loadErr == nil {
			p, loadErr = iplist.NewSnapshot(snapshot.Payload)
		}
		if loadErr != nil {
			failure := snapshotError("iplist", loadErr)
			if o.source == "iplist" {
				return engine, nil, runtimeError(failure)
			}
			engine.Providers = append(engine.Providers, unavailableProvider{source: "iplist", err: failure})
		} else {
			engine.Providers = append(engine.Providers, p)
		}
	}
	if o.source != "iplist" && includeV2fly {
		snapshot, loadErr := catalogstore.Read(dir, "v2fly")
		var provider *v2fly.SnapshotProvider
		if loadErr == nil {
			provider, loadErr = v2fly.NewSnapshot(snapshot.Payload)
		}
		if loadErr != nil {
			failure := snapshotError("v2fly", loadErr)
			if o.source == "v2fly" {
				return engine, p, runtimeError(failure)
			}
			engine.Providers = append(engine.Providers, unavailableProvider{source: "v2fly", err: failure})
		} else {
			engine.Providers = append(engine.Providers, provider)
		}
	}
	return engine, p, nil
}

func snapshotError(source string, err error) error {
	return fmt.Errorf("%s local snapshot unavailable: %w; run netCatalog update --source %s", source, err, source)
}

type unavailableProvider struct {
	source string
	err    error
}

func (p unavailableProvider) ID() string { return p.source }
func (p unavailableProvider) Capabilities() []resolver.DataKind {
	if p.source == "v2fly" {
		return []resolver.DataKind{resolver.Domains}
	}
	return []resolver.DataKind{resolver.Domains, resolver.IPs, resolver.CIDRs}
}
func (p unavailableProvider) ListServices(context.Context) ([]resolver.Service, error) {
	return nil, p.err
}
func (p unavailableProvider) Fetch(context.Context, string, resolver.Query) (resolver.Result, error) {
	return resolver.Result{}, p.err
}

func (o *options) warnings(writer io.Writer) warningPrinter {
	return warningPrinter{writer: writer, enabled: o.info, color: o.color}
}

func loadCatalogue(ctx context.Context, engine resolver.Resolver, warnings warningPrinter) (resolver.Catalogue, error) {
	catalogue := engine.ListServices(ctx)
	warnings.print(catalogue.Warnings)
	if catalogue.Succeeded == 0 && len(catalogue.Services) == 0 {
		return catalogue, runtimeError(fmt.Errorf("unable to load the local service catalogue: %v; run netCatalog update", catalogue.Warnings))
	}
	return catalogue, nil
}
