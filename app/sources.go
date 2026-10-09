package app

import (
	"fmt"

	"github.com/spf13/cobra"

	"netCatalog/internal/catalogstore"
)

const updatedAtLayout = "2006 01 02 | 15:04:05"

func newSourcesCommand(o *options) *cobra.Command {
	return &cobra.Command{
		Use:   "sources",
		Short: "Show supported data sources, data types and snapshot update time (UTC)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, err := o.catalogDir()
			if err != nil {
				return runtimeError(err)
			}
			for _, source := range []struct{ id, types string }{
				{"iplist", "domains,ips,cidrs"},
				{"v2fly", "domains"},
			} {
				updatedAt := "-"
				if snapshot, err := catalogstore.Read(dir, source.id); err == nil {
					updatedAt = snapshot.Metadata.UpdatedAt.Format(updatedAtLayout)
				}
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", source.id, source.types, updatedAt); err != nil {
					return runtimeError(err)
				}
			}
			return nil
		},
	}
}
