// Package resolver defines source-independent contracts and result merging.
package resolver

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type DataKind string

const (
	Domains DataKind = "domains"
	IPs     DataKind = "ips"
	CIDRs   DataKind = "cidrs"
)

type Family string

const (
	IPv4 Family = "ipv4"
	IPv6 Family = "ipv6"
	All  Family = "all"
)

type Query struct {
	Kind   DataKind
	Family Family
}

// Binding holds an opaque source key. Only its provider interprets the key.
type Binding struct {
	Source string `json:"source"`
	Key    string `json:"key"`
}

type Service struct {
	Domain   string    `json:"domain"`
	Bindings []Binding `json:"bindings"`
}

// Selection keeps a user-facing name separate from the discovered portals.
type Selection struct {
	Name     string
	Services []Service
}

type Entry struct {
	Value    string   `json:"value"`
	RuleType string   `json:"rule_type,omitempty"`
	Sources  []string `json:"sources"`
}

type Result struct {
	Service   string   `json:"service"`
	Portals   []string `json:"portals"`
	Kind      DataKind `json:"type"`
	Family    Family   `json:"family,omitempty"`
	Entries   []Entry  `json:"entries"`
	Warnings  []string `json:"warnings"`
	Succeeded int      `json:"successful_requests"`
}

// Provider is implemented implicitly: any type with these methods satisfies it.
type Provider interface {
	ID() string
	Capabilities() []DataKind
	ListServices(context.Context) ([]Service, error)
	Fetch(context.Context, string, Query) (Result, error)
}

type Resolver struct {
	Providers []Provider
}

func (r Resolver) Fetch(ctx context.Context, selection Selection, q Query) Result {
	result := Result{Service: selection.Name, Portals: []string{}, Kind: q.Kind, Entries: []Entry{}, Warnings: []string{}}
	for _, service := range selection.Services {
		result.Portals = append(result.Portals, service.Domain)
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

type Catalogue struct {
	Services  []Service `json:"services"`
	Warnings  []string  `json:"warnings"`
	Succeeded int       `json:"successful_requests"`
}

// ListServices discovers portals afresh and merges only exact domain identities.
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
			if domain == "" {
				continue
			}
			merged := unique[domain]
			merged.Domain = domain
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
				unique[domain] = merged
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
	sort.Slice(result.Services, func(i, j int) bool { return result.Services[i].Domain < result.Services[j].Domain })
	return result
}

func normalizeDomain(domain string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(domain), "."))
}

func (c Catalogue) Search(term string) []Service {
	term = normalizeDomain(term)
	matches := []Service{}
	for _, service := range c.Services {
		if strings.Contains(service.Domain, term) {
			matches = append(matches, service)
		}
	}
	return matches
}

func (c Catalogue) Resolve(term string) (Selection, error) {
	term = normalizeDomain(term)
	if term == "" {
		return Selection{}, fmt.Errorf("service must not be empty")
	}
	for _, service := range c.Services {
		if service.Domain == term {
			return Selection{Name: service.Domain, Services: []Service{service}}, nil
		}
	}
	// Short names select all portals whose entire first label matches.
	// This is a name heuristic, not proof that the domains share an owner.
	if !strings.Contains(term, ".") {
		matches := []Service{}
		for _, service := range c.Services {
			label, _, hasSuffix := strings.Cut(service.Domain, ".")
			if hasSuffix && label == term {
				matches = append(matches, service)
			}
		}
		if len(matches) > 0 {
			name := term
			if len(matches) == 1 {
				name = matches[0].Domain
			}
			return Selection{Name: name, Services: matches}, nil
		}
	}
	matches := c.Search(term)
	if len(matches) == 1 {
		return Selection{Name: matches[0].Domain, Services: matches}, nil
	}
	if len(matches) == 0 {
		return Selection{}, fmt.Errorf("no portal matching %q in the available catalogue", term)
	}
	domains := make([]string, 0, len(matches))
	for _, service := range matches {
		domains = append(domains, service.Domain)
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
