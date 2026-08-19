package windowsphp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
	client   *http.Client
	endpoint string
}

func New() *Provider {
	return &Provider{client: &http.Client{Timeout: 30 * time.Second}, endpoint: releasesURL}
}

type archive struct {
	Zip struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	} `json:"zip"`
}

func (p *Provider) Versions(ctx context.Context, variant, targetArch string) ([]Release, error) {
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
	var data map[string]map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
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
	return Release{}, fmt.Errorf("PHP %s is not available for %s/%s", requested, runtime.GOOS, runtime.GOARCH)
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
