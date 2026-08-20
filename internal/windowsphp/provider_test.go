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
