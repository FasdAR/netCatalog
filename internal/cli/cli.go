package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"netcraze/internal/iplist"
	"netcraze/internal/resolver"
)

const usage = `Usage:
  netcraze get SERVICE --type domains|ips|cidrs [options]
  netcraze services list [options]
  netcraze services search TERM [options]
  netcraze sources list

Options (after the command):
  --family ipv4|ipv6|all   IP family (default ipv4; not for domains)
  --url URL              iplist endpoint (default https://iplist.opencck.org/)
  --timeout DURATION     Timeout per HTTP request (default 30s)
  --site                 Query an exact iplist portal without catalogue discovery
  --info                 Show warnings on stderr (orange in a terminal)

No cache. Failed requests/invalid records are skipped. Warnings require --info.
Colors are disabled for redirected stderr, NO_COLOR, or TERM=dumb.
Exit codes: 0 success (including partial/empty results), 1 all requests failed,
2 invalid command or unknown/ambiguous service. Services are domain identities,
not source groups. Each record is printed on its own line without prefixes.
Short names (discord) combine portals with the same first label (discord.*).
Exact domains (discord.com) select only that portal.
`

// Run keeps process exit and OS streams outside the CLI so it is testable.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(stdout, usage)
		return 0
	}
	if len(args) == 2 && args[0] == "sources" && args[1] == "list" {
		_, err := fmt.Fprintln(stdout, "iplist\tdomains,ips,cidrs")
		if err != nil {
			return 1
		}
		return 0
	}
	command, term := args[0], ""
	rest := args[1:]
	switch command {
	case "get":
		if len(rest) == 0 || strings.HasPrefix(rest[0], "-") {
			fmt.Fprintln(stderr, "get requires SERVICE before flags")
			return 2
		}
		term, rest = rest[0], rest[1:]
	case "services":
		if len(rest) == 0 || (rest[0] != "list" && rest[0] != "search") {
			fmt.Fprintln(stderr, "expected services list or services search TERM")
			return 2
		}
		action := rest[0]
		rest = rest[1:]
		if action == "search" {
			if len(rest) == 0 || strings.HasPrefix(rest[0], "-") {
				fmt.Fprintln(stderr, "search requires TERM before flags")
				return 2
			}
			term, rest = rest[0], rest[1:]
		}
	default:
		fmt.Fprintln(stderr, "unknown command; use netcraze --help")
		return 2
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	baseURL := flags.String("url", iplist.DefaultURL, "HTTP source URL")
	info := flags.Bool("info", false, "show warnings on stderr")
	timeout := flags.Duration("timeout", 30*time.Second, "per-request timeout")
	var kind, family string
	var site bool
	if command == "get" {
		flags.StringVar(&kind, "type", "", "domains, ips or cidrs (required)")
		flags.StringVar(&family, "family", "ipv4", "ipv4, ipv6 or all")
		flags.BoolVar(&site, "site", false, "bypass discovery and query an exact iplist portal")
	}
	if err := flags.Parse(rest); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 || *timeout <= 0 {
		fmt.Fprintln(stderr, "invalid arguments or timeout")
		return 2
	}
	if command == "get" {
		if strings.TrimSpace(term) == "" {
			fmt.Fprintln(stderr, "SERVICE must not be empty")
			return 2
		}
		if kind != "domains" && kind != "ips" && kind != "cidrs" {
			fmt.Fprintln(stderr, "--type must be domains, ips or cidrs")
			return 2
		}
		if family != "ipv4" && family != "ipv6" && family != "all" {
			fmt.Fprintln(stderr, "--family must be ipv4, ipv6 or all")
			return 2
		}
		familySet := false
		flags.Visit(func(f *flag.Flag) { familySet = familySet || f.Name == "family" })
		if kind == "domains" && familySet {
			fmt.Fprintln(stderr, "--family does not apply to domains")
			return 2
		}
	}
	p, err := iplist.New(*baseURL, &http.Client{Timeout: *timeout})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	engine := resolver.Resolver{Providers: []resolver.Provider{p}}
	warnings := warningPrinter{writer: stderr, enabled: *info, color: supportsColor(stderr)}
	catalogue := resolver.Catalogue{Warnings: []string{}}
	if !site {
		catalogue = engine.ListServices(ctx)
		warnings.print(catalogue.Warnings)
		if catalogue.Succeeded == 0 && len(catalogue.Services) == 0 {
			fmt.Fprintln(stderr, "error: unable to load the service catalogue; use --info for details")
			return 1
		}
	}
	if command == "services" {
		filtered := catalogue.Search(term)
		for _, service := range filtered {
			if _, err = fmt.Fprintln(stdout, service.Domain); err != nil {
				break
			}
		}
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	var selection resolver.Selection
	if site {
		domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(term), "."))
		selection = resolver.Selection{Name: domain, Services: []resolver.Service{{Domain: domain, Bindings: []resolver.Binding{{Source: p.ID(), Key: domain}}}}}
	} else {
		selection, err = catalogue.Resolve(term)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
	}
	result := engine.Fetch(ctx, selection, resolver.Query{Kind: resolver.DataKind(kind), Family: resolver.Family(family)})
	warnings.print(result.Warnings)
	for _, entry := range result.Entries {
		if _, err = fmt.Fprintln(stdout, entry.Value); err != nil {
			break
		}
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if result.Succeeded == 0 {
		fmt.Fprintln(stderr, "error: all data requests failed; use --info for details")
		return 1
	}
	return 0
}
