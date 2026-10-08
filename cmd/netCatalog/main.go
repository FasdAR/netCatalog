// NetCatalog retrieves domains, IP addresses, and CIDR networks from public
// service lists. It does not modify system routes or perform DNS lookups.
package main

import (
	"context"
	"os"
	"os/signal"

	"netCatalog/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
