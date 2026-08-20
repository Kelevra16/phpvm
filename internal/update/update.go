package update

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Result struct {
	Version, CurrentPath, StagedPath string
	UpToDate                         bool
}
type release struct {
	Tag    string  `json:"tag_name"`
	Assets []asset `json:"assets"`
}
type asset struct{ Name, URL string }

func (a *asset) UnmarshalJSON(b []byte) error {
	var v struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	a.Name = v.Name
	a.URL = v.URL
	return nil
}

func Prepare(ctx context.Context, repo, want, current string) (Result, error) {
	endpoint := "https://api.github.com/repos/" + repo + "/releases/latest"
	if want != "latest" {
		if !strings.HasPrefix(want, "v") {
			want = "v" + want
		}
		endpoint = "https://api.github.com/repos/" + repo + "/releases/tags/" + want
	}
	client := &http.Client{Timeout: 60 * time.Second}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	req.Header.Set("User-Agent", "phpvm-self-update")
	resp, err := client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("release lookup returned %s", resp.Status)
	}
	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return Result{}, err
	}
	version := strings.TrimPrefix(rel.Tag, "v")
	if current == version || current == rel.Tag {
		return Result{Version: version, UpToDate: true}, nil
	}
	arch := "amd64"
	if runtime.GOARCH == "386" {
		arch = "386"
	}
	archiveName := "phpvm_" + version + "_windows_" + arch + ".zip"
	var archive, checks asset
	for _, a := range rel.Assets {
		if a.Name == archiveName {
			archive = a
		}
		if a.Name == "checksums.txt" {
			checks = a
		}
	}
	if archive.URL == "" || checks.URL == "" {
		return Result{}, fmt.Errorf("release %s lacks required Windows assets", rel.Tag)
	}
	archiveData, err := download(ctx, client, archive.URL)
	if err != nil {
		return Result{}, err
	}
	manifest, err := download(ctx, client, checks.URL)
	if err != nil {
		return Result{}, err
	}
	expected := ""
	for _, line := range strings.Split(string(manifest), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[len(fields)-1] == archiveName {
			expected = fields[0]
			break
		}
	}
	sum := sha256.Sum256(archiveData)
	if expected == "" || !strings.EqualFold(expected, hex.EncodeToString(sum[:])) {
		return Result{}, fmt.Errorf("release checksum verification failed")
	}
	exe, err := os.Executable()
	if err != nil {
		return Result{}, err
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return Result{}, err
	}
	stage := exe + ".new"
	if err := extractExecutable(archiveData, stage); err != nil {
		return Result{}, err
	}
	return Result{Version: version, CurrentPath: exe, StagedPath: stage}, nil
}
func download(ctx context.Context, c *http.Client, url string) ([]byte, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("User-Agent", "phpvm-self-update")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}
func extractExecutable(data []byte, dest string) error {
	tmp, err := os.CreateTemp("", "phpvm-update-*.zip")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	z, err := zip.OpenReader(name)
	if err != nil {
		return err
	}
	defer z.Close()
	for _, f := range z.File {
		if filepath.Base(f.Name) != "phpvm.exe" {
			continue
		}
		src, err := f.Open()
		if err != nil {
			return err
		}
		dst, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
		if err != nil {
			src.Close()
			return err
		}
		_, cpErr := io.Copy(dst, src)
		src.Close()
		closeErr := dst.Close()
		if cpErr != nil {
			return cpErr
		}
		return closeErr
	}
	return fmt.Errorf("release archive does not contain phpvm.exe")
}
func Schedule(r Result) error {
	pid := os.Getpid()
	script := fmt.Sprintf("Wait-Process -Id %d -ErrorAction SilentlyContinue; Move-Item -LiteralPath %s -Destination %s -Force", pid, psQuote(r.StagedPath), psQuote(r.CurrentPath))
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", script)
	return cmd.Start()
}
func psQuote(v string) string { return "'" + strings.ReplaceAll(v, "'", "''") + "'" }
