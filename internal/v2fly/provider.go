// Package v2fly reads the upstream compiled YAML release, not raw source files.
package v2fly

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
	"netcraze/internal/resolver"
)

const DefaultURL = "https://github.com/v2fly/domain-list-community/releases/latest/download/dlc.dat_plain.yml"
const maxBody = 32 << 20

type list struct {
	Name   string   `yaml:"name"`
	Length int      `yaml:"length"`
	Rules  []string `yaml:"rules"`
}

type Provider struct {
	url    string
	client *http.Client
	// A provider lives for one CLI invocation. This snapshot avoids downloading
	// the same release again for each selected list; nothing is persisted.
	once  sync.Once
	lists map[string]list
	err   error
}

func New(rawURL string, client *http.Client) (*Provider, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Fragment != "" {
		return nil, fmt.Errorf("invalid v2fly HTTP URL: %q", rawURL)
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &Provider{url: rawURL, client: client}, nil
}

func (*Provider) ID() string                        { return "v2fly" }
func (*Provider) Capabilities() []resolver.DataKind { return []resolver.DataKind{resolver.Domains} }

func (p *Provider) load(ctx context.Context) error {
	p.once.Do(func() { p.err = p.download(ctx) })
	return p.err
}

func (p *Provider) download(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "netcraze/0.1")
	response, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	if err != nil {
		return err
	}
	if len(body) > maxBody {
		return fmt.Errorf("release exceeds %d MiB", maxBody>>20)
	}
	var document struct {
		Lists []list `yaml:"lists"`
	}
	decoder := yaml.NewDecoder(bytes.NewReader(body))
	decoder.KnownFields(true)
	if err := decoder.Decode(&document); err != nil {
		return fmt.Errorf("invalid release YAML: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("expected a single YAML document")
	}
	if document.Lists == nil {
		return fmt.Errorf("release YAML is missing lists")
	}
	p.lists = make(map[string]list)
	for _, item := range document.Lists {
		name := strings.ToLower(strings.TrimSpace(item.Name))
		if strings.HasPrefix(name, "category") || strings.Contains(name, "-") {
			continue
		}
		if name == "" || strings.ContainsAny(name, " \t\r\n:/@") {
			return fmt.Errorf("invalid list name %q", item.Name)
		}
		if _, exists := p.lists[name]; exists {
			return fmt.Errorf("duplicate list %q", name)
		}
		if item.Length != len(item.Rules) {
			return fmt.Errorf("list %q has inconsistent length", name)
		}
		p.lists[name] = item
	}
	return nil
}

func (p *Provider) ListServices(ctx context.Context) ([]resolver.Service, error) {
	if err := p.load(ctx); err != nil {
		return nil, err
	}
	services := []resolver.Service{}
	for name := range p.lists {
		services = append(services, resolver.Service{Name: name, Bindings: []resolver.Binding{{Source: p.ID(), Key: name}}})
	}
	sort.Slice(services, func(i, j int) bool { return services[i].Name < services[j].Name })
	return services, nil
}

func (p *Provider) Fetch(ctx context.Context, key string, q resolver.Query) (resolver.Result, error) {
	result := resolver.Result{Entries: []resolver.Entry{}, Warnings: []string{}}
	if q.Kind != resolver.Domains {
		return result, fmt.Errorf("only domains are supported")
	}
	if err := p.load(ctx); err != nil {
		return result, err
	}
	item, exists := p.lists[key]
	if !exists {
		return result, fmt.Errorf("unknown or excluded list %q", key)
	}
	result.Succeeded = 1
	ignored, invalid := 0, 0
	for _, rule := range item.Rules {
		if !strings.HasPrefix(rule, "domain:") {
			ignored++
			continue
		}
		value := strings.TrimPrefix(rule, "domain:")
		// The release encodes attributes as :@attr (not part of the domain).
		value, _, _ = strings.Cut(value, ":")
		value = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
		if !validDomain(value) {
			invalid++
			continue
		}
		result.Entries = append(result.Entries, resolver.Entry{Value: value, RuleType: "suffix"})
	}
	if ignored > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("v2fly %s: ignored %d non-domain rules (including full, keyword, regexp)", key, ignored))
	}
	if invalid > 0 {
		result.Warnings = append(result.Warnings, fmt.Sprintf("v2fly %s: skipped %d invalid domains", key, invalid))
	}
	return result, nil
}

func validDomain(value string) bool {
	if value == "" || len(value) > 253 {
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
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}
