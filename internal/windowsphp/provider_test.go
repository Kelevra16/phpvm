package windowsphp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCompare(t *testing.T) {
	if compare("8.4.10", "8.4.9") <= 0 {
		t.Fatal("expected 8.4.10 to be newer")
	}
	if compare("8.3.1", "8.4.0") >= 0 {
		t.Fatal("expected 8.3.1 to be older")
	}
}

func TestRegistryCache(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"8.4":{"version":"8.4.1","nts-vs17-x64":{"zip":{"path":"php.zip","sha256":"abc"}}}}`))
	}))
	p := New(t.TempDir())
	p.endpoint = srv.URL
	p.archiveEndpoint = "://invalid"
	p.cacheTTL = time.Hour
	if _, err := p.Versions(context.Background(), "nts", "x64"); err != nil {
		t.Fatal(err)
	}
	srv.Close()
	if _, err := p.Versions(context.Background(), "nts", "x64"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("registry called %d times", calls)
	}
}

func TestParseArchiveIndex(t *testing.T) {
	html := []byte(`<a href="php-5.6.40-nts-Win32-VC11-x64.zip">nts</a>
<a href="php-5.6.40-Win32-VC11-x64.zip">ts</a>
<a href="php-5.4.45-nts-Win32-VC9-x86.zip">x86</a>
<a href="php-debug-pack-5.6.40-nts-Win32-VC11-x64.zip">debug</a>`)
	r := parseArchiveIndex(html, archivesURL, "nts", "x64")
	if len(r) != 1 || r[0].Version != "5.6.40" || !r[0].Archived || r[0].SHA256 != "" {
		t.Fatalf("unexpected archive releases: %#v", r)
	}
}

func TestComposerConstraints(t *testing.T) {
	cases := []struct {
		version, constraint string
		want                bool
	}{{"8.4.2", "^8.3", true}, {"9.0.0", "^8.3", false}, {"8.4.2", ">=8.2 <8.5", true}, {"8.5.0", ">=8.2 <8.5", false}, {"8.4.2", "8.4.*", true}, {"8.3.9", "~8.3.2", true}, {"8.4.0", "~8.3.2", false}}
	for _, tc := range cases {
		if got := satisfies(tc.version, tc.constraint); got != tc.want {
			t.Errorf("satisfies(%s,%s)=%v want %v", tc.version, tc.constraint, got, tc.want)
		}
	}
}
