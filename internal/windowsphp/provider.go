package windowsphp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const releasesURL = "https://windows.php.net/downloads/releases/releases.json"
const downloadBaseURL = "https://windows.php.net/downloads/releases/"

type Release struct {
	Version string `json:"version"`
	Variant string `json:"variant"`
	Arch    string `json:"arch"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
}
type Provider struct {
	client    *http.Client
	endpoint  string
	cachePath string
	cacheTTL  time.Duration
}

func New(cacheRoot ...string) *Provider {
	p := &Provider{client: &http.Client{Timeout: 30 * time.Second}, endpoint: releasesURL, cacheTTL: 6 * time.Hour}
	if len(cacheRoot) > 0 && cacheRoot[0] != "" {
		p.cachePath = filepath.Join(cacheRoot[0], "cache", "windows-releases.json")
	}
	if v := os.Getenv("PHPVM_CACHE_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			p.cacheTTL = d
		}
	}
	return p
}

type archive struct {
	Zip struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	} `json:"zip"`
}

func (p *Provider) Versions(ctx context.Context, variant, targetArch string) ([]Release, error) {
	payload, err := p.registry(ctx)
	if err != nil {
		return nil, err
	}
	var data map[string]map[string]json.RawMessage
	if err := json.NewDecoder(bytes.NewReader(payload)).Decode(&data); err != nil {
		return nil, fmt.Errorf("decode release registry: %w", err)
	}
	out := make([]Release, 0, len(data))
	if variant == "" {
		variant = "nts"
	}
	if targetArch == "" {
		targetArch = "x64"
		if runtime.GOARCH == "386" {
			targetArch = "x86"
		}
	}
	archSuffix := "-" + targetArch
	for _, fields := range data {
		var version string
		if raw, ok := fields["version"]; ok {
			_ = json.Unmarshal(raw, &version)
		}
		var a archive
		for name, raw := range fields {
			if strings.HasPrefix(name, variant+"-") && strings.HasSuffix(name, archSuffix) {
				_ = json.Unmarshal(raw, &a)
				break
			}
		}
		if version == "" || a.Zip.Path == "" {
			continue
		}
		url := a.Zip.Path
		if !strings.HasPrefix(url, "http") {
			url = downloadBaseURL + strings.TrimPrefix(url, "/")
		}
		out = append(out, Release{Version: version, Variant: variant, Arch: targetArch, URL: url, SHA256: a.Zip.SHA256})
	}
	sort.Slice(out, func(i, j int) bool { return compare(out[i].Version, out[j].Version) > 0 })
	return out, nil
}

func (p *Provider) registry(ctx context.Context) ([]byte, error) {
	if p.cachePath != "" {
		if st, err := os.Stat(p.cachePath); err == nil && time.Since(st.ModTime()) < p.cacheTTL {
			if b, err := os.ReadFile(p.cachePath); err == nil {
				return b, nil
			}
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("release registry returned %s", resp.Status)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if p.cachePath != "" {
		if err := os.MkdirAll(filepath.Dir(p.cachePath), 0755); err == nil {
			_ = os.WriteFile(p.cachePath, b, 0644)
		}
	}
	return b, nil
}

func (p *Provider) Resolve(ctx context.Context, requested, variant, arch string) (Release, error) {
	versions, err := p.Versions(ctx, variant, arch)
	if err != nil {
		return Release{}, err
	}
	if requested == "latest" {
		if len(versions) == 0 {
			return Release{}, fmt.Errorf("no PHP releases available")
		}
		return versions[0], nil
	}
	for _, r := range versions {
		if r.Version == requested {
			return r, nil
		}
	}
	// A minor selector such as 8.4 resolves to its newest patch.
	for _, r := range versions {
		if strings.HasPrefix(r.Version, requested+".") {
			return r, nil
		}
	}
	if strings.ContainsAny(requested, "^~*<>=|") || strings.Contains(requested, " ") {
		for _, r := range versions {
			if satisfies(r.Version, requested) {
				return r, nil
			}
		}
	}
	return Release{}, fmt.Errorf("PHP %s is not available for %s/%s", requested, runtime.GOOS, runtime.GOARCH)
}

type semver struct{ major, minor, patch int }

func parseSemver(v string) semver {
	var s semver
	fmt.Sscanf(strings.TrimSpace(strings.TrimPrefix(v, "v")), "%d.%d.%d", &s.major, &s.minor, &s.patch)
	return s
}
func cmpSemver(a, b semver) int {
	if a.major != b.major {
		if a.major < b.major {
			return -1
		}
		return 1
	}
	if a.minor != b.minor {
		if a.minor < b.minor {
			return -1
		}
		return 1
	}
	if a.patch != b.patch {
		if a.patch < b.patch {
			return -1
		}
		return 1
	}
	return 0
}
func satisfies(version, constraint string) bool {
	v := parseSemver(version)
	for _, orPart := range strings.Split(constraint, "||") {
		ok := true
		tokens := strings.Fields(strings.ReplaceAll(orPart, ",", " "))
		for i := 0; i < len(tokens); i++ {
			token := tokens[i]
			if (token == ">=" || token == "<=" || token == ">" || token == "<" || token == "=") && i+1 < len(tokens) {
				i++
				token += tokens[i]
			}
			if !satisfiesToken(v, token) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}
func satisfiesToken(v semver, token string) bool {
	token = strings.TrimSpace(token)
	if token == "" || token == "*" {
		return true
	}
	op := "="
	for _, candidate := range []string{">=", "<=", ">", "<", "^", "~", "="} {
		if strings.HasPrefix(token, candidate) {
			op = candidate
			token = strings.TrimSpace(strings.TrimPrefix(token, candidate))
			break
		}
	}
	if strings.ContainsAny(token, "*xX") {
		parts := strings.Split(token, ".")
		if len(parts) > 0 && parts[0] != "*" && parts[0] != "x" && parts[0] != "X" {
			var major int
			fmt.Sscanf(parts[0], "%d", &major)
			if v.major != major {
				return false
			}
		}
		if len(parts) > 1 && parts[1] != "*" && parts[1] != "x" && parts[1] != "X" {
			var minor int
			fmt.Sscanf(parts[1], "%d", &minor)
			if v.minor != minor {
				return false
			}
		}
		return true
	}
	target := parseSemver(token)
	c := cmpSemver(v, target)
	switch op {
	case ">=":
		return c >= 0
	case "<=":
		return c <= 0
	case ">":
		return c > 0
	case "<":
		return c < 0
	case "^":
		upper := semver{major: target.major + 1}
		if target.major == 0 {
			upper = semver{minor: target.minor + 1}
		}
		return c >= 0 && cmpSemver(v, upper) < 0
	case "~":
		upper := semver{major: target.major + 1}
		if strings.Count(token, ".") >= 2 {
			upper = semver{major: target.major, minor: target.minor + 1}
		}
		return c >= 0 && cmpSemver(v, upper) < 0
	default:
		if strings.Count(token, ".") == 1 {
			return v.major == target.major && v.minor == target.minor
		}
		return c == 0
	}
}

func compare(a, b string) int {
	var aa, bb [3]int
	fmt.Sscanf(a, "%d.%d.%d", &aa[0], &aa[1], &aa[2])
	fmt.Sscanf(b, "%d.%d.%d", &bb[0], &bb[1], &bb[2])
	for i := range aa {
		if aa[i] < bb[i] {
			return -1
		}
		if aa[i] > bb[i] {
			return 1
		}
	}
	return 0
}
