package app

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/Kelevra16/phpvm/internal/store"
	"github.com/Kelevra16/phpvm/internal/update"
)

func (a *App) which(s *store.Store, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("usage: phpvm which [build]")
	}
	id, err := s.Current()
	if len(args) == 1 {
		id, err = resolveInstalled(s, args[0])
	}
	if err != nil {
		return err
	}
	path := s.Executable(id)
	if _, err := os.Stat(path); err != nil {
		return err
	}
	fmt.Fprintln(a.Out, path)
	return nil
}

func (a *App) cache(s *store.Store, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: phpvm cache <dir|list|verify|clear>")
	}
	path := filepath.Join(s.Root, "cache")
	switch args[0] {
	case "dir":
		fmt.Fprintln(a.Out, path)
		return nil
	case "clear":
		if err := os.RemoveAll(path); err != nil {
			return err
		}
		fmt.Fprintln(a.Out, "Cleared", path)
		return nil
	case "list":
		entries, _ := filepath.Glob(filepath.Join(path, "archives", "*.zip"))
		sort.Strings(entries)
		for _, entry := range entries {
			st, err := os.Stat(entry)
			if err == nil {
				fmt.Fprintf(a.Out, "%s\t%d bytes\n", strings.TrimSuffix(filepath.Base(entry), ".zip"), st.Size())
			}
		}
		return nil
	case "verify":
		entries, _ := filepath.Glob(filepath.Join(path, "archives", "*.zip"))
		for _, entry := range entries {
			got, err := hashLocalFile(entry)
			if err != nil || !strings.EqualFold(got, strings.TrimSuffix(filepath.Base(entry), ".zip")) {
				return fmt.Errorf("invalid cached archive %s", filepath.Base(entry))
			}
		}
		fmt.Fprintf(a.Out, "Verified %d cached archives\n", len(entries))
		return nil
	default:
		return fmt.Errorf("usage: phpvm cache <dir|list|verify|clear>")
	}
}

type bundleManifest struct {
	Schema int               `json:"schema"`
	Files  map[string]string `json:"files"`
}

func (a *App) bundle(s *store.Store, args []string) error {
	if len(args) != 2 || (args[0] != "create" && args[0] != "import") {
		return fmt.Errorf("usage: phpvm bundle <create|import> <file.zip>")
	}
	path, err := filepath.Abs(args[1])
	if err != nil {
		return err
	}
	if args[0] == "create" {
		return a.createBundle(s, path)
	}
	return a.importBundle(s, path)
}

func (a *App) createBundle(s *store.Store, destination string) error {
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("create bundle (destination must not exist): %w", err)
	}
	remove := true
	defer func() {
		if remove {
			_ = os.Remove(destination)
		}
	}()
	zw := zip.NewWriter(out)
	manifest := bundleManifest{Schema: 1, Files: map[string]string{}}
	cacheRoot := filepath.Join(s.Root, "cache")
	var files []string
	_ = filepath.Walk(cacheRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr == nil && !info.IsDir() {
			rel, relErr := filepath.Rel(cacheRoot, path)
			if relErr == nil && (rel == "windows-releases.json" || rel == "windows-archives.html" || rel == "archive-index.json" || strings.HasPrefix(filepath.ToSlash(rel), "archives/")) {
				files = append(files, path)
			}
		}
		return nil
	})
	sort.Strings(files)
	for _, path := range files {
		rel, _ := filepath.Rel(cacheRoot, path)
		name := filepath.ToSlash(rel)
		digest, err := hashLocalFile(path)
		if err != nil {
			zw.Close()
			out.Close()
			return err
		}
		manifest.Files[name] = digest
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		if err != nil {
			zw.Close()
			out.Close()
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			zw.Close()
			out.Close()
			return err
		}
		_, copyErr := io.Copy(w, in)
		closeErr := in.Close()
		if copyErr != nil {
			zw.Close()
			out.Close()
			return copyErr
		}
		if closeErr != nil {
			zw.Close()
			out.Close()
			return closeErr
		}
	}
	mw, err := zw.Create("phpvm-bundle.json")
	if err == nil {
		err = json.NewEncoder(mw).Encode(manifest)
	}
	if err == nil {
		err = zw.Close()
	} else {
		_ = zw.Close()
	}
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	remove = false
	fmt.Fprintf(a.Out, "Created bundle %s with %d files\n", destination, len(files))
	return nil
}

func (a *App) importBundle(s *store.Store, source string) error {
	zr, err := zip.OpenReader(source)
	if err != nil {
		return err
	}
	defer zr.Close()
	entries := map[string]*zip.File{}
	var manifest bundleManifest
	for _, f := range zr.File {
		name := filepath.ToSlash(filepath.Clean(f.Name))
		if name != f.Name || strings.HasPrefix(name, "../") || strings.Contains(name, ":") {
			return fmt.Errorf("unsafe bundle path %q", f.Name)
		}
		if name == "phpvm-bundle.json" {
			r, err := f.Open()
			if err != nil {
				return err
			}
			err = json.NewDecoder(io.LimitReader(r, 1<<20)).Decode(&manifest)
			r.Close()
			if err != nil {
				return err
			}
		} else {
			entries[name] = f
		}
	}
	if manifest.Schema != 1 || len(manifest.Files) == 0 {
		return fmt.Errorf("invalid or empty phpvm bundle manifest")
	}
	cacheRoot := filepath.Join(s.Root, "cache")
	stage, err := os.MkdirTemp(s.Root, ".bundle-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	for name, expected := range manifest.Files {
		f, ok := entries[name]
		if !ok {
			return fmt.Errorf("bundle entry %s is missing", name)
		}
		if name != "windows-releases.json" && name != "windows-archives.html" && name != "archive-index.json" && !strings.HasPrefix(name, "archives/") {
			return fmt.Errorf("unsupported bundle entry %s", name)
		}
		target := filepath.Join(stage, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		r, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			r.Close()
			return err
		}
		h := sha256.New()
		_, copyErr := io.Copy(io.MultiWriter(out, h), io.LimitReader(r, 1<<30))
		closeOut := out.Close()
		closeIn := r.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeOut != nil {
			return closeOut
		}
		if closeIn != nil {
			return closeIn
		}
		if !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), expected) {
			return fmt.Errorf("bundle checksum mismatch for %s", name)
		}
		if strings.HasPrefix(name, "archives/") && !strings.EqualFold(strings.TrimSuffix(filepath.Base(name), ".zip"), expected) {
			return fmt.Errorf("archive name/checksum mismatch for %s", name)
		}
	}
	if err := os.MkdirAll(cacheRoot, 0755); err != nil {
		return err
	}
	for name := range manifest.Files {
		sourcePath := filepath.Join(stage, filepath.FromSlash(name))
		target := filepath.Join(cacheRoot, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		if err := os.Rename(sourcePath, target); err != nil {
			if os.IsExist(err) {
				continue
			}
			_ = os.Remove(target)
			if err := os.Rename(sourcePath, target); err != nil {
				return err
			}
		}
	}
	fmt.Fprintf(a.Out, "Imported %d cached files\n", len(manifest.Files))
	return nil
}

func hashLocalFile(path string) (string, error) {
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

func (a *App) selfUpdate(ctx context.Context, args []string) error {
	if len(args) > 1 {
		return fmt.Errorf("usage: phpvm self-update [version]")
	}
	version := "latest"
	if len(args) == 1 {
		version = args[0]
	}
	if runtime.GOOS != "windows" {
		return fmt.Errorf("self-update currently supports Windows only")
	}
	result, err := update.Prepare(ctx, "Kelevra16/phpvm", version, a.Version)
	if err != nil {
		return err
	}
	if result.UpToDate {
		fmt.Fprintln(a.Out, "phpvm", result.Version, "is already current")
		return nil
	}
	if err := update.Schedule(result); err != nil {
		return err
	}
	fmt.Fprintln(a.Out, "Prepared phpvm", result.Version+"; the executable will be replaced after this command exits")
	return nil
}
