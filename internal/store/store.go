package store

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Metadata struct {
	Version          string    `json:"version"`
	Variant          string    `json:"variant"`
	Arch             string    `json:"arch"`
	URL              string    `json:"url"`
	ArchiveSHA256    string    `json:"archiveSha256"`
	ExecutableSHA256 string    `json:"executableSha256"`
	InstalledAt      time.Time `json:"installedAt"`
	Imported         bool      `json:"imported,omitempty"`
	ValidationError  string    `json:"validationError,omitempty"`
	ValidatedAt      time.Time `json:"validatedAt,omitempty"`
	Runtime          string    `json:"runtime,omitempty"`
	SourceKind       string    `json:"sourceKind,omitempty"`
}

// Import copies an existing PHP distribution into managed storage.
func (s *Store) Import(source string, m Metadata) error {
	return s.WithLock(context.Background(), func() error {
		if s.IsInstalled(m.ID()) {
			return fmt.Errorf("PHP build %s is already installed", m.ID())
		}
		if err := os.MkdirAll(s.versionsDir(), 0755); err != nil {
			return err
		}
		stage, err := os.MkdirTemp(s.versionsDir(), ".import-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(stage)
		if err := copyTree(source, stage); err != nil {
			return err
		}
		php := filepath.Join(stage, "php.exe")
		m.ExecutableSHA256, err = fileHash(php)
		if err != nil {
			return fmt.Errorf("source does not contain php.exe: %w", err)
		}
		m.Imported = true
		m.InstalledAt = time.Now().UTC()
		b, _ := json.MarshalIndent(m, "", "  ")
		if err := os.WriteFile(filepath.Join(stage, "phpvm.json"), append(b, '\n'), 0644); err != nil {
			return err
		}
		return renameWithRetry(stage, s.installation(m.ID()))
	})
}

func copyTree(source, destination string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
		if err != nil {
			return err
		}
		_, cpErr := io.Copy(out, in)
		closeErr := out.Close()
		if cpErr != nil {
			return cpErr
		}
		return closeErr
	})
}

func (m Metadata) ID() string { return m.Version + "-" + m.Variant + "-" + m.Arch }

type Store struct {
	Root     string
	Progress func(downloaded, total int64)
	Validate func(context.Context, string) error
}

func New(root string) *Store                   { return &Store{Root: root, Validate: validatePHP} }
func (s *Store) versionsDir() string           { return filepath.Join(s.Root, "versions") }
func (s *Store) currentFile() string           { return filepath.Join(s.Root, "current") }
func (s *Store) installation(id string) string { return filepath.Join(s.versionsDir(), id) }
func (s *Store) Executable(id string) string   { return filepath.Join(s.installation(id), "php.exe") }
func (s *Store) IsInstalled(id string) bool {
	st, err := os.Stat(s.Executable(id))
	return err == nil && !st.IsDir()
}

func (s *Store) Installed() ([]Metadata, error) {
	entries, err := os.ReadDir(s.versionsDir())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Metadata
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		st, statErr := os.Stat(filepath.Join(s.versionsDir(), e.Name()))
		if statErr != nil || !st.IsDir() {
			continue
		}
		m, err := s.Metadata(e.Name())
		if err == nil {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out, nil
}

func (s *Store) Metadata(id string) (Metadata, error) {
	var m Metadata
	b, err := os.ReadFile(filepath.Join(s.installation(id), "phpvm.json"))
	if err != nil {
		return m, err
	}
	err = json.Unmarshal(b, &m)
	return m, err
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
func (s *Store) Use(id string) error {
	return s.WithLock(context.Background(), func() error { return s.useUnlocked(id) })
}
func (s *Store) useUnlocked(id string) error {
	if !s.IsInstalled(id) {
		return fmt.Errorf("PHP build %s is not installed", id)
	}
	wrappers := map[string]string{
		"php.cmd":    dynamicWrapper("php"),
		"phpize.cmd": dynamicWrapper("phpize"),
	}
	for name, body := range wrappers {
		if err := atomicWrite(filepath.Join(s.Root, "bin", name), []byte(body)); err != nil {
			return err
		}
	}
	return atomicWrite(s.currentFile(), []byte(id+"\n"))
}

func dynamicWrapper(tool string) string {
	return "@echo off\r\nset \"PHPVM_TOOL_PATH=\"\r\nfor /f \"usebackq delims=\" %%P in (`phpvm resolve --path --tool " + tool + "`) do set \"PHPVM_TOOL_PATH=%%P\"\r\nif not defined PHPVM_TOOL_PATH exit /b 1\r\n\"%PHPVM_TOOL_PATH%\" %*\r\n"
}

func (s *Store) Install(ctx context.Context, m Metadata) error {
	return s.WithLock(ctx, func() error { return s.installUnlocked(ctx, m) })
}
func (s *Store) installUnlocked(ctx context.Context, m Metadata) error {
	if s.IsInstalled(m.ID()) {
		return nil
	}
	if err := os.MkdirAll(s.versionsDir(), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.Root, "php-*.zip")
	if err != nil {
		return err
	}
	archivePath := tmp.Name()
	defer os.Remove(archivePath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.URL, nil)
	if err != nil {
		tmp.Close()
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		tmp.Close()
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		tmp.Close()
		return fmt.Errorf("download returned %s", resp.Status)
	}
	h := sha256.New()
	pw := &progressWriter{total: resp.ContentLength, fn: s.Progress}
	if _, err := io.Copy(io.MultiWriter(tmp, h, pw), resp.Body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if m.ArchiveSHA256 != "" && !strings.EqualFold(got, m.ArchiveSHA256) {
		return fmt.Errorf("checksum mismatch: got %s", got)
	}
	if m.ArchiveSHA256 == "" {
		// Historical Windows builds do not publish adjacent SHA-256 values.
		// Record the observed hash so repair and metadata remain reproducible.
		m.ArchiveSHA256 = got
	}
	stage, err := os.MkdirTemp(s.versionsDir(), ".install-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := unzip(archivePath, stage); err != nil {
		return err
	}
	php := filepath.Join(stage, "php.exe")
	if _, err := os.Stat(php); err != nil {
		return fmt.Errorf("download does not contain php.exe: %w", err)
	}
	m.ExecutableSHA256, err = fileHash(php)
	if err != nil {
		return err
	}
	if s.Validate != nil {
		if err := s.Validate(ctx, php); err != nil {
			return fmt.Errorf("validate staged PHP: %w", err)
		}
		m.ValidatedAt = time.Now().UTC()
	}
	m.InstalledAt = time.Now().UTC()
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(filepath.Join(stage, "phpvm.json"), append(b, '\n'), 0644); err != nil {
		return err
	}
	if err := renameWithRetry(stage, s.installation(m.ID())); err != nil {
		return fmt.Errorf("publish installation: %w", err)
	}
	return nil
}

func validatePHP(ctx context.Context, php string) error {
	for _, args := range [][]string{{"--version"}, {"--ini"}, {"-m"}} {
		cmd := exec.CommandContext(ctx, php, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			detail := strings.TrimSpace(string(out))
			if detail == "" {
				detail = err.Error()
			}
			return fmt.Errorf("php %s failed: %s", strings.Join(args, " "), detail)
		}
	}
	return nil
}

type progressWriter struct {
	downloaded, total int64
	fn                func(int64, int64)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	p.downloaded += int64(len(b))
	if p.fn != nil {
		p.fn(p.downloaded, p.total)
	}
	return len(b), nil
}

func (s *Store) Verify(id string) error {
	m, err := s.Metadata(id)
	if err != nil {
		return err
	}
	got, err := fileHash(s.Executable(id))
	if err != nil {
		return err
	}
	if !strings.EqualFold(got, m.ExecutableSHA256) {
		return fmt.Errorf("php.exe checksum mismatch")
	}
	return nil
}
func (s *Store) Repair(ctx context.Context, id string) error {
	return s.WithLock(ctx, func() error { return s.repairUnlocked(ctx, id) })
}
func (s *Store) repairUnlocked(ctx context.Context, id string) error {
	m, err := s.Metadata(id)
	if err != nil {
		return err
	}
	dest := s.installation(id)
	backup := filepath.Join(s.versionsDir(), ".repair-"+id)
	_ = os.RemoveAll(backup)
	if err := os.Rename(dest, backup); err != nil {
		return err
	}
	if err := s.installUnlocked(ctx, m); err != nil {
		_ = os.Rename(backup, dest)
		return err
	}
	return os.RemoveAll(backup)
}
func (s *Store) Uninstall(id string) error {
	return s.WithLock(context.Background(), func() error { return s.uninstallUnlocked(id) })
}
func (s *Store) uninstallUnlocked(id string) error {
	current, _ := s.Current()
	if current == id {
		return fmt.Errorf("cannot remove active build %s", id)
	}
	if !s.IsInstalled(id) {
		return fmt.Errorf("PHP build %s is not installed", id)
	}
	return os.RemoveAll(s.installation(id))
}
func (s *Store) Prune() ([]string, error) {
	var removed []string
	err := s.WithLock(context.Background(), func() error { var err error; removed, err = s.pruneUnlocked(); return err })
	return removed, err
}
func (s *Store) pruneUnlocked() ([]string, error) {
	current, err := s.Current()
	if err != nil {
		return nil, err
	}
	builds, err := s.Installed()
	if err != nil {
		return nil, err
	}
	var removed []string
	for _, m := range builds {
		if m.ID() != current {
			if err := os.RemoveAll(s.installation(m.ID())); err != nil {
				return removed, err
			}
			removed = append(removed, m.ID())
		}
	}
	return removed, nil
}
func (s *Store) Clean() ([]string, error) {
	var removed []string
	err := s.WithLock(context.Background(), func() error { var err error; removed, err = s.cleanUnlocked(); return err })
	return removed, err
}
func (s *Store) cleanUnlocked() ([]string, error) {
	entries, err := os.ReadDir(s.versionsDir())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var removed []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".install-") {
			p := filepath.Join(s.versionsDir(), e.Name())
			if err := os.RemoveAll(p); err != nil {
				return removed, err
			}
			removed = append(removed, p)
		}
	}
	return removed, nil
}

func (s *Store) WithLock(ctx context.Context, fn func() error) error {
	release, err := s.lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	return fn()
}
func (s *Store) lock(ctx context.Context) (func(), error) {
	if err := os.MkdirAll(s.Root, 0755); err != nil {
		return nil, err
	}
	path := filepath.Join(s.Root, ".lock")
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			fmt.Fprintf(f, "%d\n%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339Nano))
			f.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if staleLock(path) {
			_ = os.Remove(path)
			continue
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("waiting for another phpvm process: %w", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}
func staleLock(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	line := strings.SplitN(string(b), "\n", 2)[0]
	pid, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		return true
	}
	return !processAlive(pid)
}
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if runtime.GOOS == "windows" {
		out, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH").Output()
		return err == nil && strings.Contains(string(out), fmt.Sprintf("\"%d\"", pid))
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(os.Signal(nil)) == nil
}
func atomicWrite(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".write-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	_ = os.Remove(path)
	return os.Rename(name, path)
}
func renameWithRetry(oldPath, newPath string) error {
	var err error
	for attempt := 0; attempt < 20; attempt++ {
		if err = os.Rename(oldPath, newPath); err == nil {
			return nil
		}
		time.Sleep(time.Duration(attempt+1) * 25 * time.Millisecond)
	}
	return err
}
func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func unzip(path, dest string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer r.Close()
	clean := filepath.Clean(dest) + string(os.PathSeparator)
	for _, f := range r.File {
		target := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), clean) {
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
