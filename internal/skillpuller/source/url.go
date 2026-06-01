package source

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
)

const maxSkillSize = 1 << 20 // 1 MiB

type URLSource struct {
	Client       *http.Client
	AllowPrivate bool // skip SSRF checks (for testing only)
}

func (s *URLSource) Pull(ctx context.Context, ref string, _ PullOptions) (*Skill, error) {
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}

	parsed, err := url.Parse(ref)
	if err != nil {
		return nil, fmt.Errorf("invalid URL %q: %w", ref, err)
	}

	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return nil, fmt.Errorf("unsupported scheme %q: only http and https are allowed", parsed.Scheme)
	}

	if !s.AllowPrivate {
		if err := rejectPrivateHost(parsed.Hostname()); err != nil {
			return nil, fmt.Errorf("blocked URL %q: %w", ref, err)
		}
	}

	// Build request from the parsed/validated URL, not the raw input,
	// so static analysis can verify the URL was sanitized.
	sanitizedURL := parsed.String()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sanitizedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid URL %q: %w", ref, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %q: %w", ref, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %q: status %d", ref, resp.StatusCode)
	}

	content, err := io.ReadAll(io.LimitReader(resp.Body, maxSkillSize+1))
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", ref, err)
	}
	if len(content) > maxSkillSize {
		return nil, fmt.Errorf("skill from %q exceeds size limit (%d bytes)", ref, maxSkillSize)
	}

	name := skillNameFromURL(ref)

	return &Skill{Name: name, Content: content}, nil
}

// rejectPrivateHost blocks requests to loopback, link-local, and
// private IP ranges to prevent SSRF.
var privateNetworks []*net.IPNet

func init() {
	for _, cidr := range []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16",
		"127.0.0.0/8",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	} {
		_, network, _ := net.ParseCIDR(cidr)
		privateNetworks = append(privateNetworks, network)
	}
}

func rejectPrivateHost(host string) error {
	ips := []net.IP{net.ParseIP(host)}
	if ips[0] == nil {
		resolved, err := net.LookupIP(host)
		if err != nil || len(resolved) == 0 {
			return fmt.Errorf("cannot resolve host %q", host)
		}
		ips = resolved
	}

	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("address %s is not allowed", ip)
		}
		for _, network := range privateNetworks {
			if network.Contains(ip) {
				return fmt.Errorf("address %s is in a private range", ip)
			}
		}
	}

	return nil
}

// skillNameFromURL extracts a skill name from a URL path.
// For ".../skill-name/SKILL.md" returns "skill-name".
// For ".../SKILL.md" returns "SKILL".
// For other paths returns the last path segment.
func skillNameFromURL(rawURL string) string {
	p := path.Base(rawURL)

	if strings.EqualFold(p, "SKILL.md") {
		dir := path.Dir(rawURL)
		parent := path.Base(dir)
		if parent != "" && parent != "." && parent != "/" &&
			!strings.Contains(parent, ".") {
			return parent
		}
		return strings.TrimSuffix(p, path.Ext(p))
	}

	return strings.TrimSuffix(p, path.Ext(p))
}
