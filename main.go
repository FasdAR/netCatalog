// NetCatalog retrieves service domains, IP addresses and CIDR networks.
package main

import (
	"context"
	"os"
	"os/signal"

	"netCatalog/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := app.Execute(ctx)
	stop()
	os.Exit(code)
}
