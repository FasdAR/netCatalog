package iplist

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"netCatalog/internal/resolver"
)

// SnapshotProvider reads a complete iplist export without network access.
// Construct it with NewSnapshot; the parsed data is immutable.
type SnapshotProvider struct {
	sites   map[string]map[string][]string
	keys    map[string]string
	invalid int
}

// NewSnapshot validates a full JSON export, including all five data arrays.
// Invalid portal identities are skipped; malformed usable portal schemas fail.
func NewSnapshot(payload []byte) (*SnapshotProvider, error) {
	if len(payload) > maxBody {
		return nil, fmt.Errorf("iplist export exceeds %d MiB", maxBody>>20)
	}
	body := bytes.TrimSpace(payload)
	if bytes.Equal(body, []byte("[]")) {
		body = []byte("{}")
	}
	if len(body) == 0 || body[0] != '{' {
		return nil, fmt.Errorf("expected a JSON object")
	}
	var sites map[string]json.RawMessage
	if err := json.Unmarshal(body, &sites); err != nil {
		return nil, fmt.Errorf("invalid iplist JSON: %w", err)
	}
	p := &SnapshotProvider{sites: make(map[string]map[string][]string), keys: make(map[string]string)}
	for key, raw := range sites {
		domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(key), "."))
		raw = bytes.TrimSpace(raw)
		if !validDomain(domain) || len(raw) == 0 || raw[0] != '{' {
			p.invalid++
			continue
		}
		var record map[string]json.RawMessage
		if err := json.Unmarshal(raw, &record); err != nil {
			return nil, err
		}
		fields := make(map[string][]string)
		for _, field := range []string{"domains", "ip4", "ip6", "cidr4", "cidr6"} {
			value := bytes.TrimSpace(record[field])
			if len(value) == 0 || value[0] != '[' {
				return nil, fmt.Errorf("iplist %s: missing or invalid %s array", key, field)
			}
			var items []string
			if err := json.Unmarshal(value, &items); err != nil {
				return nil, fmt.Errorf("iplist %s %s: %w", key, field, err)
			}
			fields[field] = items
		}
		if _, exists := p.keys[domain]; exists {
			return nil, fmt.Errorf("duplicate normalized iplist portal %q", domain)
		}
		p.sites[key] = fields
		p.keys[domain] = key
	}
	return p, nil
}

// ID returns the source identifier.
func (*SnapshotProvider) ID() string { return "iplist" }

// Capabilities returns the data kinds available in an iplist export.
func (*SnapshotProvider) Capabilities() []resolver.DataKind {
	return []resolver.DataKind{resolver.Domains, resolver.IPs, resolver.CIDRs}
}

// ListServices returns sorted portals and a diagnostic for skipped identities.
func (p *SnapshotProvider) ListServices(ctx context.Context) ([]resolver.Service, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	services := []resolver.Service{}
	for domain, key := range p.keys {
		services = append(services, resolver.Service{Domain: domain, Bindings: []resolver.Binding{{Source: p.ID(), Key: key}}})
	}
	sort.Slice(services, func(i, j int) bool { return services[i].Domain < services[j].Domain })
	if p.invalid > 0 {
		return services, fmt.Errorf("skipped %d invalid catalogue records", p.invalid)
	}
	return services, nil
}

// Fetch normalizes records from exactly one local portal. Each selected field
// counts as a successful logical read, including empty arrays.
func (p *SnapshotProvider) Fetch(ctx context.Context, key string, q resolver.Query) (resolver.Result, error) {
	r := resolver.Result{Entries: []resolver.Entry{}, Warnings: []string{}}
	if err := ctx.Err(); err != nil {
		return r, err
	}
	site, found := p.sites[key]
	if !found {
		return r, fmt.Errorf("unknown iplist portal %q", key)
	}
	fields := []string{"domains"}
	if q.Kind != resolver.Domains {
		prefix := "ip"
		if q.Kind == resolver.CIDRs {
			prefix = "cidr"
		} else if q.Kind != resolver.IPs {
			return r, fmt.Errorf("unsupported data type %q", q.Kind)
		}
		switch q.Family {
		case resolver.IPv4:
			fields = []string{prefix + "4"}
		case resolver.IPv6:
			fields = []string{prefix + "6"}
		case resolver.All:
			fields = []string{prefix + "4", prefix + "6"}
		default:
			return r, fmt.Errorf("unsupported family %q", q.Family)
		}
	}
	if q.Kind == resolver.CIDRs {
		r.Warnings = append(r.Warnings, "iplist: JSON exports raw CIDRs without upstream replace substitutions; networks may include unrelated services")
	}
	for _, field := range fields {
		invalid := 0
		for _, value := range site[field] {
			if err := ctx.Err(); err != nil {
				return r, err
			}
			entry, err := Normalize(value, field)
			if err != nil {
				invalid++
				continue
			}
			r.Entries = append(r.Entries, entry)
		}
		r.Succeeded++
		if invalid > 0 {
			r.Warnings = append(r.Warnings, fmt.Sprintf("iplist %s: skipped %d invalid records", field, invalid))
		}
	}
	return r, nil
}
