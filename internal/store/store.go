package store

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Store struct{ Root string }

func New(root string) *Store         { return &Store{Root: root} }
func (s *Store) versionsDir() string { return filepath.Join(s.Root, "versions") }
func (s *Store) currentFile() string { return filepath.Join(s.Root, "current") }
func (s *Store) IsInstalled(v string) bool {
	st, err := os.Stat(filepath.Join(s.versionsDir(), v, "php.exe"))
	return err == nil && !st.IsDir()
}

func (s *Store) Installed() ([]string, error) {
	entries, err := os.ReadDir(s.versionsDir())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && s.IsInstalled(e.Name()) {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}
func (s *Store) Current() (string, error) {
	b, err := os.ReadFile(s.currentFile())
	if os.IsNotExist(err) {
		return "", fmt.Errorf("no active PHP version")
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}
func (s *Store) Use(v string) error {
	if !s.IsInstalled(v) {
		return fmt.Errorf("PHP %s is not installed", v)
	}
	if err := os.MkdirAll(filepath.Join(s.Root, "bin"), 0755); err != nil {
		return err
	}
	wrappers := map[string]string{
		"php.cmd":    "@echo off\r\n\"%~dp0..\\versions\\" + v + "\\php.exe\" %*\r\n",
		"phpize.cmd": "@echo off\r\n\"%~dp0..\\versions\\" + v + "\\phpize.bat\" %*\r\n",
	}
	for name, body := range wrappers {
		if err := os.WriteFile(filepath.Join(s.Root, "bin", name), []byte(body), 0644); err != nil {
			return err
		}
	}
	return os.WriteFile(s.currentFile(), []byte(v+"\n"), 0644)
}
func (s *Store) Install(ctx context.Context, v, url, wantHash string) error {
	if err := os.MkdirAll(s.versionsDir(), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.Root, "php-*.zip")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		tmp.Close()
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		tmp.Close()
		return err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		tmp.Close()
		return fmt.Errorf("download returned %s", resp.Status)
	}
	h := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(tmp, h), resp.Body)
	resp.Body.Close()
	closeErr := tmp.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	got := hex.EncodeToString(h.Sum(nil))
	if wantHash != "" && !strings.EqualFold(got, wantHash) {
		return fmt.Errorf("checksum mismatch: got %s", got)
	}
	dest := filepath.Join(s.versionsDir(), v)
	if err := os.Mkdir(dest, 0755); err != nil {
		return err
	}
	if err := unzip(tmpPath, dest); err != nil {
		os.RemoveAll(dest)
		return err
	}
	return nil
}
func unzip(path, dest string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer r.Close()
	cleanDest := filepath.Clean(dest) + string(os.PathSeparator)
	for _, f := range r.File {
		target := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), cleanDest) {
			return fmt.Errorf("invalid zip path %q", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		src, err := f.Open()
		if err != nil {
			return err
		}
		dst, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
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
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}
func (s *Store) Uninstall(v string) error {
	current, _ := s.Current()
	if current == v {
		return fmt.Errorf("cannot remove active PHP %s; activate another version first", v)
	}
	path := filepath.Join(s.versionsDir(), v)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("PHP %s is not installed", v)
	}
	return os.RemoveAll(path)
}
func (s *Store) Prune() ([]string, error) {
	current, err := s.Current()
	if err != nil {
		return nil, err
	}
	versions, err := s.Installed()
	if err != nil {
		return nil, err
	}
	var removed []string
	for _, v := range versions {
		if v != current {
			if err := os.RemoveAll(filepath.Join(s.versionsDir(), v)); err != nil {
				return removed, err
			}
			removed = append(removed, v)
		}
	}
	return removed, nil
}
