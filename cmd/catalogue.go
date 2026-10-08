package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"netCatalog/internal/providers/iplist"
	"netCatalog/internal/providers/v2fly"
	"netCatalog/internal/resolver"
)

func (o *options) engine(includeV2fly bool) (resolver.Resolver, *iplist.Provider, error) {
	client := &http.Client{Timeout: o.timeout}
	p, err := iplist.New(o.iplistURL, client)
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
