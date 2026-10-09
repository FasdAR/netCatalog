// Package resolver defines source-independent contracts and result merging.
package resolver

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// DataKind identifies the requested category of network records.
type DataKind string

// Supported categories of network records.
const (
	Domains DataKind = "domain" // Domain rules.
	IPs     DataKind = "ip"     // Individual IP addresses.
	CIDRs   DataKind = "cidr"   // Network prefixes in CIDR notation.
)

// Family selects IP versions for IP and CIDR queries; it is ignored for domains.
type Family string

// Supported address-family selections.
const (
	IPv4 Family = "ipv4" // IPv4 only.
	IPv6 Family = "ipv6" // IPv6 only.
	All  Family = "all"  // Both IP versions.
)

// Query specifies the data kind and, for IP or CIDR records, the address family.
type Query struct {
	Kind   DataKind // Requested record category.
	Family Family   // Requested IP versions; ignored for Domains.
}

// Binding holds an opaque source key. Only its provider interprets the key.
type Binding struct {
	Source string `json:"source"` // Provider ID.
	Key    string `json:"key"`    // Source-specific identifier, not a search term.
}

// Service is a discovered domain portal or named list. A named list need not
// have a domain; the resolver does not infer one from its name.
type Service struct {
	Name     string    `json:"name,omitempty"` // Source-reported list name, if any.
	Domain   string    `json:"domain"`         // Domain identity, if provided.
	Bindings []Binding `json:"bindings"`       // Sources that can fetch this service.
}

// Identifier returns the domain if present, otherwise the list name.
func (s Service) Identifier() string {
	if s.Domain != "" {
		return s.Domain
	}
	return s.Name
}

// Selection keeps a user-facing name separate from the discovered portals.
type Selection struct {
	Name     string    // User-facing name of the selection.
	Services []Service // Selected domain portals and named lists.
}

// Entry is a normalized domain, IP address, or network prefix. Domain rule types
// remain distinct during merging even when their values match.
type Entry struct {
	Value    string   `json:"value"`               // Normalized record value.
	RuleType string   `json:"rule_type,omitempty"` // Domain semantics, such as suffix or full.
	Sources  []string `json:"sources"`             // Provider IDs; populated by Resolver.
}

// Result contains usable records and diagnostics from a fetch. Provider results
// need only set Entries, Warnings, and Succeeded; Resolver adds selection metadata
// and provenance. A result can be partial or successfully empty.
type Result struct {
	Service   string   `json:"service"`             // User-facing selection name.
	Portals   []string `json:"portals"`             // Selected domain identities, sorted and unique.
	Kind      DataKind `json:"type"`                // Requested record category.
	Family    Family   `json:"family,omitempty"`    // Requested IP versions; empty for Domains.
	Entries   []Entry  `json:"entries"`             // Usable records, merged and sorted by Resolver.
	Warnings  []string `json:"warnings"`            // Failed requests and skipped-record diagnostics.
	Succeeded int      `json:"successful_requests"` // Successful logical requests, including empty ones.
}

// Provider discovers services and retrieves normalized records using opaque
// source keys. Implementations own all interpretation of those keys.
type Provider interface {
	// ID returns a stable source identifier used by Binding.Source.
	ID() string
	// Capabilities returns the data kinds supported by Fetch.
	Capabilities() []DataKind
	// ListServices may return usable services alongside an error for partial failure.
	ListServices(context.Context) ([]Service, error)
	// Fetch reports successful logical requests and warnings in Result.
	// Usable entries may accompany an error; callers retain partial results.
	Fetch(context.Context, string, Query) (Result, error)
}

// Resolver discovers and combines services from its configured providers.
// Its zero value has no sources and returns empty results.
type Resolver struct {
	Providers []Provider // Sources in discovery and fetch order.
}

// Fetch retrieves supported records for selection, fetching each source binding
// at most once. It retains partial results, records failures as warnings, and
// deduplicates by rule type and value while preserving sorted source provenance.
// It returns Succeeded == 0 if no logical requests succeeded.
func (r Resolver) Fetch(ctx context.Context, selection Selection, q Query) Result {
	result := Result{Service: selection.Name, Portals: []string{}, Kind: q.Kind, Entries: []Entry{}, Warnings: []string{}}
	for _, service := range selection.Services {
		if service.Domain != "" {
			result.Portals = append(result.Portals, service.Domain)
		}
	}
	sort.Strings(result.Portals)
	result.Portals = compact(result.Portals)
	if q.Kind != Domains {
		result.Family = q.Family
	}
	unique := make(map[string]Entry)
	fetched := make(map[Binding]bool)
	for _, p := range r.Providers {
		supported := false
		for _, kind := range p.Capabilities() {
			supported = supported || kind == q.Kind
		}
		if !supported {
			continue
		}
		for _, service := range selection.Services {
			for _, binding := range service.Bindings {
				if binding.Source != p.ID() || fetched[binding] {
					continue
				}
				fetched[binding] = true
				part, err := p.Fetch(ctx, binding.Key, q)
				if err != nil {
					result.Warnings = append(result.Warnings, fmt.Sprintf("%s %s: %v", p.ID(), binding.Key, err))
				}
				result.Succeeded += part.Succeeded
				result.Warnings = append(result.Warnings, part.Warnings...)
				for _, entry := range part.Entries {
					key := entry.RuleType + "\x00" + entry.Value
					previous, exists := unique[key]
					if !exists {
						previous = Entry{Value: entry.Value, RuleType: entry.RuleType}
					}
					previous.Sources = append(previous.Sources, p.ID())
					unique[key] = previous
				}
			}
		}
	}
	for _, entry := range unique {
		sort.Strings(entry.Sources)
		entry.Sources = compact(entry.Sources)
		result.Entries = append(result.Entries, entry)
	}
	sort.Slice(result.Entries, func(i, j int) bool {
		if result.Entries[i].Value == result.Entries[j].Value {
			return result.Entries[i].RuleType < result.Entries[j].RuleType
		}
		return result.Entries[i].Value < result.Entries[j].Value
	})
	return result
}

// Catalogue is a snapshot of discovered domain portals and named lists.
type Catalogue struct {
	Services  []Service `json:"services"`            // Services sorted by Identifier.
	Warnings  []string  `json:"warnings"`            // Source discovery diagnostics.
	Succeeded int       `json:"successful_requests"` // Sources returning success or usable partial records.
}

// ListServices asks every provider for its catalogue and retains partial results.
// It normalizes identities, merges exact matching domains or named-list names,
// and retains unique bindings belonging to their reporting provider. It does not
// merge a named list with a domain merely because their names resemble each other.
func (r Resolver) ListServices(ctx context.Context) Catalogue {
	result := Catalogue{Services: []Service{}, Warnings: []string{}}
	unique := make(map[string]Service)
	for _, p := range r.Providers {
		services, err := p.ListServices(ctx)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("%s catalogue: %v", p.ID(), err))
		}
		// A provider can return usable records alongside a warning about skipped ones.
		if err == nil || len(services) > 0 {
			result.Succeeded++
		}
		for _, service := range services {
			domain := normalizeDomain(service.Domain)
			name := normalizeDomain(service.Name)
			if domain == "" && name == "" {
				continue
			}
			key := "domain:" + domain
			if domain == "" {
				key = "name:" + name
			}
			merged := unique[key]
			merged.Domain = domain
			merged.Name = name
			for _, binding := range service.Bindings {
				if binding.Source != p.ID() || binding.Key == "" {
					continue
				}
				found := false
				for _, existing := range merged.Bindings {
					found = found || existing == binding
				}
				if !found {
					merged.Bindings = append(merged.Bindings, binding)
				}
			}
			if len(merged.Bindings) > 0 {
				unique[key] = merged
			}
		}
	}
	for _, service := range unique {
		sort.Slice(service.Bindings, func(i, j int) bool {
			if service.Bindings[i].Source == service.Bindings[j].Source {
				return service.Bindings[i].Key < service.Bindings[j].Key
			}
			return service.Bindings[i].Source < service.Bindings[j].Source
		})
		result.Services = append(result.Services, service)
	}
	sort.Slice(result.Services, func(i, j int) bool { return result.Services[i].Identifier() < result.Services[j].Identifier() })
	return result
}

func normalizeDomain(domain string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
}

// Search returns services whose identifiers contain the normalized term.
// Matching is case-insensitive and ignores a trailing dot and surrounding spaces.
// An empty term returns all services; opaque binding keys are never searched.
func (c Catalogue) Search(term string) []Service {
	term = normalizeDomain(term)
	matches := []Service{}
	for _, service := range c.Services {
		if strings.Contains(service.Identifier(), term) {
			matches = append(matches, service)
		}
	}
	return matches
}

// Resolve selects an exact domain first. A short name then selects all portals
// with that entire first label and named lists with that name. This is a name
// heuristic, not proof of shared ownership. Otherwise a unique substring match
// is selected. Empty, missing, or ambiguous terms return an error.
func (c Catalogue) Resolve(term string) (Selection, error) {
	term = normalizeDomain(term)
	if term == "" {
		return Selection{}, fmt.Errorf("service must not be empty")
	}
	for _, service := range c.Services {
		if service.Domain == term && strings.Contains(term, ".") {
			return Selection{Name: service.Domain, Services: []Service{service}}, nil
		}
	}
	// Short names select all portals whose entire first label matches.
	// This is a name heuristic, not proof that the domains share an owner.
	if !strings.Contains(term, ".") {
		matches := []Service{}
		for _, service := range c.Services {
			label, _, hasSuffix := strings.Cut(service.Domain, ".")
			if (hasSuffix && label == term) || (service.Domain == "" && service.Name == term) || service.Domain == term {
				matches = append(matches, service)
			}
		}
		if len(matches) > 0 {
			name := term
			if len(matches) == 1 {
				name = matches[0].Identifier()
			}
			return Selection{Name: name, Services: matches}, nil
		}
	}
	matches := c.Search(term)
	if len(matches) == 1 {
		return Selection{Name: matches[0].Identifier(), Services: matches}, nil
	}
	if len(matches) == 0 {
		return Selection{}, fmt.Errorf("no portal matching %q in the available catalogue", term)
	}
	domains := make([]string, 0, len(matches))
	for _, service := range matches {
		domains = append(domains, service.Identifier())
	}
	return Selection{}, fmt.Errorf("ambiguous service %q; specify a domain: %s", term, strings.Join(domains, ", "))
}

func compact(items []string) []string {
	out := items[:0]
	for _, item := range items {
		if len(out) == 0 || out[len(out)-1] != item {
			out = append(out, item)
		}
	}
	return out
}
