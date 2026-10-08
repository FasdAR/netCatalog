// Package iplist adapts the public iplist HTTP API to resolver contracts.
package iplist

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strings"

	"netCatalog/internal/resolver"
)

// DefaultURL is the public iplist HTTP endpoint used by the CLI.
const DefaultURL = "https://iplist.opencck.org/"
const maxBody = 64 << 20

// Provider adapts an iplist HTTP endpoint to [resolver.Provider]. Every call
// obtains fresh data; no responses are cached. The zero value is not usable;
// create a Provider with [New].
type Provider struct {
	base   *url.URL
	client *http.Client
}

// New creates a provider for an HTTP or HTTPS baseURL without a fragment.
// It returns an error for invalid URLs. A nil client uses [http.DefaultClient];
// callers should supply a client with a timeout when bounded requests are needed.
func New(baseURL string, client *http.Client) (*Provider, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Fragment != "" {
		return nil, fmt.Errorf("invalid HTTP source URL: %q", baseURL)
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &Provider{base: u, client: client}, nil
}

// ID returns the source identifier used in catalogue bindings and provenance.
func (*Provider) ID() string { return "iplist" }

// Capabilities returns the supported domain, IP, and CIDR data kinds.
func (*Provider) Capabilities() []resolver.DataKind {
	return []resolver.DataKind{resolver.Domains, resolver.IPs, resolver.CIDRs}
}

func (p *Provider) request(ctx context.Context, params url.Values, target any) (err error) {
	u := *p.base
	values := u.Query()
	// Selection must come only from the current command, not from a base URL.
	for _, key := range []string{"format", "data", "group", "group[]", "site", "site[]"} {
		values.Del(key)
	}
	values.Set("format", "json")
	for key, vals := range params {
		values[key] = vals
	}
	u.RawQuery = values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "netCatalog/0.1")
	response, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, response.Body.Close())
	}()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	if err != nil {
		return err
	}
	if len(body) > maxBody {
		return fmt.Errorf("response exceeds %d MiB", maxBody>>20)
	}
	trimmed := bytes.TrimSpace(body)
	// PHP serializes an empty associative array as [], not {}.
	if bytes.Equal(trimmed, []byte("[]")) {
		trimmed = []byte("{}")
	}
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return fmt.Errorf("expected a JSON object")
	}
	if err := json.Unmarshal(trimmed, target); err != nil {
		return fmt.Errorf("invalid JSON response: %w", err)
	}
	return nil
}

// ListServices downloads the full catalogue and returns sorted domain portals
// with their source bindings. Source groups are not services. Invalid records
// are skipped; usable services may be returned together with an error describing
// skipped records. HTTP, size-limit, and decoding failures return an error.
func (p *Provider) ListServices(ctx context.Context) ([]resolver.Service, error) {
	var sites map[string]json.RawMessage
	if err := p.request(ctx, nil, &sites); err != nil {
		return nil, err
	}
	services := []resolver.Service{}
	invalid := 0
	for name, metadata := range sites {
		domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
		metadata = bytes.TrimSpace(metadata)
		if !validDomain(domain) || len(metadata) == 0 || metadata[0] != '{' {
			invalid++
			continue
		}
		// The JSON object key is iplist's portal selector. Group is not identity.
		services = append(services, resolver.Service{Domain: domain, Bindings: []resolver.Binding{{Source: p.ID(), Key: name}}})
	}
	sort.Slice(services, func(i, j int) bool { return services[i].Domain < services[j].Domain })
	if invalid > 0 {
		return services, fmt.Errorf("skipped %d invalid catalogue records", invalid)
	}
	return services, nil
}

// Fetch requests the portal key for q. IPv4 and IPv6 requests are independent;
// failed requests and invalid records are reported in Result.Warnings, while
// successful requests contribute to Result.Succeeded, even if empty. A complete
// HTTP failure is therefore represented by Succeeded == 0, not a non-nil error.
// Invalid keys or queries return an error. Each response is limited to 64 MiB.
// CIDRs are masked but not aggregated and retain upstream's raw network scope.
func (p *Provider) Fetch(ctx context.Context, key string, q resolver.Query) (resolver.Result, error) {
	result := resolver.Result{Entries: []resolver.Entry{}, Warnings: []string{}}
	if strings.TrimSpace(key) == "" {
		return result, fmt.Errorf("portal key must not be empty")
	}
	fields := []string{"domains"}
	if q.Kind != resolver.Domains {
		prefix := "ip"
		if q.Kind == resolver.CIDRs {
			prefix = "cidr"
		} else if q.Kind != resolver.IPs {
			return result, fmt.Errorf("unsupported data type %q", q.Kind)
		}
		switch q.Family {
		case resolver.IPv4:
			fields = []string{prefix + "4"}
		case resolver.IPv6:
			fields = []string{prefix + "6"}
		case resolver.All:
			fields = []string{prefix + "4", prefix + "6"}
		default:
			return result, fmt.Errorf("unsupported family %q", q.Family)
		}
	}
	if q.Kind == resolver.CIDRs {
		result.Warnings = append(result.Warnings, "iplist: JSON exports raw CIDRs without upstream replace substitutions; networks may include unrelated services")
	}
	for _, field := range fields {
		params := url.Values{"data": {field}}
		params.Set("site", key)
		var sites map[string][]string
		if err := p.request(ctx, params, &sites); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("iplist %s: %v", field, err))
			continue
		}
		result.Succeeded++
		if len(sites) == 0 {
			result.Warnings = append(result.Warnings, fmt.Sprintf("iplist %s: no matching site %q", field, key))
		}
		invalid := 0
		for _, items := range sites {
			for _, item := range items {
				entry, err := normalize(item, field)
				if err != nil {
					invalid++
					continue
				}
				result.Entries = append(result.Entries, entry)
			}
		}
		if invalid > 0 {
			result.Warnings = append(result.Warnings, fmt.Sprintf("iplist %s: skipped %d invalid records", field, invalid))
		}
	}
	return result, nil
}

func normalize(value, field string) (resolver.Entry, error) {
	value = strings.TrimSpace(value)
	entry := resolver.Entry{}
	if field == "domains" {
		value = strings.ToLower(strings.TrimSuffix(value, "."))
		value = strings.TrimPrefix(value, "*.")
		if !validDomain(value) {
			return entry, fmt.Errorf("invalid domain")
		}
		// iplist's default domain routing semantics are suffix matching.
		return resolver.Entry{Value: value, RuleType: "suffix"}, nil
	}
	var address netip.Addr
	if strings.HasPrefix(field, "cidr") {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return entry, err
		}
		address = prefix.Addr()
		entry.Value = prefix.Masked().String()
	} else {
		var err error
		address, err = netip.ParseAddr(value)
		if err != nil || address.Zone() != "" {
			return entry, fmt.Errorf("invalid IP address")
		}
		entry.Value = address.String()
	}
	if address.Is4In6() || address.Is4() != strings.HasSuffix(field, "4") {
		return resolver.Entry{}, fmt.Errorf("wrong address family")
	}
	return entry, nil
}

func validDomain(value string) bool {
	if len(value) == 0 || len(value) > 253 {
		return false
	}
	if _, err := netip.ParseAddr(value); err == nil {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' {
				continue
			}
			return false
		}
	}
	return true
}
