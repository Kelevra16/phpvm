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
	shim := filepath.Join(filepath.Dir(r.CurrentPath), "php.exe")
	shimBackup, shimHadPrevious, err := replaceFromCopy(r.StagedPath, shim)
	if err != nil {
		return fmt.Errorf("install native PHP shim: %w", err)
	}
	backup := r.CurrentPath + ".old"
	_ = os.Remove(backup)
	if err := os.Rename(r.CurrentPath, backup); err != nil {
		rollbackReplacement(shim, shimBackup, shimHadPrevious)
		return fmt.Errorf("stage current executable: %w", err)
	}
	if err := os.Rename(r.StagedPath, r.CurrentPath); err != nil {
		_ = os.Rename(backup, r.CurrentPath)
		rollbackReplacement(shim, shimBackup, shimHadPrevious)
		return fmt.Errorf("activate updated executable: %w", err)
	}
	return nil
}

func replaceFromCopy(source, target string) (string, bool, error) {
	stage := target + ".new"
	backup := target + ".old"
	_ = os.Remove(stage)
	_ = os.Remove(backup)
	in, err := os.Open(source)
	if err != nil {
		return backup, false, err
	}
	out, err := os.OpenFile(stage, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
	if err != nil {
		in.Close()
		return backup, false, err
	}
	_, copyErr := io.Copy(out, in)
	closeInErr := in.Close()
	closeOutErr := out.Close()
	if copyErr != nil || closeInErr != nil || closeOutErr != nil {
		_ = os.Remove(stage)
		if copyErr != nil {
			return backup, false, copyErr
		}
		if closeInErr != nil {
			return backup, false, closeInErr
		}
		return backup, false, closeOutErr
	}
	hadPrevious := false
	if _, err := os.Stat(target); err == nil {
		hadPrevious = true
		if err := os.Rename(target, backup); err != nil {
			_ = os.Remove(stage)
			return backup, false, err
		}
	} else if !os.IsNotExist(err) {
		_ = os.Remove(stage)
		return backup, false, err
	}
	if err := os.Rename(stage, target); err != nil {
		if hadPrevious {
			_ = os.Rename(backup, target)
		}
		return backup, false, err
	}
	return backup, hadPrevious, nil
}

func rollbackReplacement(target, backup string, hadPrevious bool) {
	_ = os.Remove(target)
	if hadPrevious {
		_ = os.Rename(backup, target)
	}
}

// EnsurePHPShim provisions the native launcher for release binaries and
// refreshes it after migration from an updater that only replaced phpvm.exe.
// Development builds are left untouched to avoid modifying source folders.
func EnsurePHPShim(version string) error {
	if version == "" || strings.EqualFold(version, "dev") {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return ensurePHPShim(exe)
}

func ensurePHPShim(exe string) error {
	if !strings.EqualFold(filepath.Base(exe), "phpvm.exe") {
		return nil
	}
	shim := filepath.Join(filepath.Dir(exe), "php.exe")
	_, markerErr := os.Stat(exe + ".old")
	_, shimErr := os.Stat(shim)
	if os.IsNotExist(markerErr) && shimErr == nil {
		return nil
	}
	if markerErr != nil && !os.IsNotExist(markerErr) {
		return markerErr
	}
	if shimErr != nil && !os.IsNotExist(shimErr) {
		return shimErr
	}
	_, _, err := replaceFromCopy(exe, shim)
	return err
}

// CleanupPrevious removes the executable retained during the last atomic swap.
func CleanupPrevious() {
	exe, err := os.Executable()
	if err == nil {
		_ = os.Remove(exe + ".old")
		_ = os.Remove(filepath.Join(filepath.Dir(exe), "php.exe.old"))
	}
}
