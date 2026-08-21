package app

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
	"sort"
	"strings"
	"time"

	"github.com/Kelevra16/phpvm/internal/store"
	"github.com/Kelevra16/phpvm/internal/windowsphp"
)

type projectLock struct {
	Schema         int               `json:"schema"`
	PHP            store.Metadata    `json:"php"`
	INI            map[string]string `json:"ini,omitempty"`
	Extensions     []string          `json:"extensions,omitempty"`
	ComposerSHA256 string            `json:"composerSha256,omitempty"`
	Profile        string            `json:"profile,omitempty"`
	Logs           map[string]string `json:"logs,omitempty"`
}

func (a *App) info(ctx context.Context, s *store.Store, args []string) error {
	wantsJSON := false
	clean := args[:0]
	for _, arg := range args {
		if arg == "--json" {
			wantsJSON = true
		} else {
			clean = append(clean, arg)
		}
	}
	args = clean
	o, rest, err := parseBuildFlags("info", args)
	if err != nil || len(rest) != 1 {
		return fmt.Errorf("usage: phpvm info [--ts] [--arch x64|x86] [--json] <version>")
	}
	p, err := provider(s.Root)
	if err != nil {
		return err
	}
	r, err := p.Resolve(ctx, rest[0], o.variant, o.arch)
	if err != nil {
		return err
	}
	data := map[string]any{"version": r.Version, "variant": r.Variant, "arch": r.Arch, "url": r.URL, "archived": r.Archived, "eol": windowsphp.IsEOL(r.Version, time.Now()), "compilerRuntime": windowsphp.CompilerRuntime(r.Version), "officialChecksum": r.SHA256 != "", "installed": s.IsInstalled(r.Version + "-" + r.Variant + "-" + r.Arch)}
	if o.json || wantsJSON {
		return json.NewEncoder(a.Out).Encode(data)
	}
	keys := []string{"version", "variant", "arch", "compilerRuntime", "archived", "eol", "officialChecksum", "installed", "url"}
	for _, k := range keys {
		fmt.Fprintf(a.Out, "%-18s %v\n", k+":", data[k])
	}
	return nil
}

func (a *App) supported(ctx context.Context, s *store.Store, args []string) error {
	return a.remote(ctx, append(args, "--supported-only"))
}

func (a *App) runtimeInfo(args []string) error {
	if len(args) != 2 || args[0] != "info" {
		return fmt.Errorf("usage: phpvm runtime info <VC6|VC9|VC11|VC14|VS16|VS17>")
	}
	name := strings.ToUpper(args[1])
	urls := map[string]string{"VC6": "https://learn.microsoft.com/en-us/cpp/windows/latest-supported-vc-redist", "VC9": "https://learn.microsoft.com/en-us/cpp/windows/latest-supported-vc-redist", "VC11": "https://www.microsoft.com/download/details.aspx?id=30679", "VC14": "https://aka.ms/vs/17/release/vc_redist.x64.exe", "VS16": "https://aka.ms/vs/17/release/vc_redist.x64.exe", "VS17": "https://aka.ms/vs/17/release/vc_redist.x64.exe"}
	url, ok := urls[name]
	if !ok {
		return fmt.Errorf("unknown runtime %s", name)
	}
	fmt.Fprintln(a.Out, "Runtime:", name)
	fmt.Fprintln(a.Out, "Microsoft:", url)
	return nil
}

func (a *App) lockProject(s *store.Store, args []string) error {
	check := len(args) == 1 && args[0] == "--check"
	if len(args) > 1 || (len(args) == 1 && !check) {
		return fmt.Errorf("usage: phpvm lock [--check]")
	}
	l, err := buildProjectLock(s)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if check {
		existing, e := os.ReadFile("phpvm.lock")
		if e != nil {
			return e
		}
		if string(existing) != string(b) {
			return fmt.Errorf("phpvm.lock is out of date")
		}
		fmt.Fprintln(a.Out, "phpvm.lock is current")
		return nil
	}
	if err := os.WriteFile("phpvm.lock", b, 0644); err != nil {
		return err
	}
	fmt.Fprintln(a.Out, "Locked", l.PHP.ID(), "to phpvm.lock")
	return nil
}

func buildProjectLock(s *store.Store) (projectLock, error) {
	id, err := resolveRuntimeBuild(s, "")
	if err != nil {
		return projectLock{}, err
	}
	m, err := s.Metadata(id)
	if err != nil {
		return projectLock{}, err
	}
	dir := filepath.Dir(s.Executable(id))
	ini, _ := readINI(filepath.Join(dir, "php.ini"))
	enabled, _ := enabledExtensions(filepath.Join(dir, "php.ini"))
	ext := make([]string, 0, len(enabled))
	for name := range enabled {
		ext = append(ext, name)
	}
	sort.Strings(ext)
	l := projectLock{Schema: 2, PHP: m, INI: ini, Extensions: ext, Logs: map[string]string{}}
	if b, e := os.ReadFile(filepath.Join(s.Root, "active-profile")); e == nil {
		l.Profile = strings.TrimSpace(string(b))
	}
	if cfg, e := findProjectConfig(); e == nil {
		l.Logs["scope"] = cfg.LogScope
		l.Logs["path"] = cfg.LogPath
	}
	if sum, e := hashFile(filepath.Join(s.Root, "tools", "composer.phar")); e == nil {
		l.ComposerSHA256 = sum
	}
	return l, nil
}

func (a *App) restoreProject(ctx context.Context, s *store.Store, args []string) error {
	allow, dry, frozen := false, false, false
	for _, arg := range args {
		switch arg {
		case "--allow-unverified-archive":
			allow = true
		case "--dry-run":
			dry = true
		case "--frozen":
			frozen = true
		default:
			return fmt.Errorf("unknown restore option %s", arg)
		}
	}
	var l projectLock
	b, err := os.ReadFile("phpvm.lock")
	if err != nil {
		return err
	}
	if err = json.Unmarshal(b, &l); err != nil {
		return err
	}
	if l.Schema < 1 || l.Schema > 2 {
		return fmt.Errorf("unsupported phpvm.lock schema %d", l.Schema)
	}
	if dry {
		fmt.Fprintln(a.Out, "Would restore", l.PHP.ID())
		fmt.Fprintln(a.Out, "INI entries:", len(l.INI))
		fmt.Fprintln(a.Out, "Extensions:", strings.Join(l.Extensions, ", "))
		return nil
	}
	if frozen {
		if !s.IsInstalled(l.PHP.ID()) {
			return fmt.Errorf("frozen restore requires installed build %s", l.PHP.ID())
		}
		m, e := s.Metadata(l.PHP.ID())
		if e != nil {
			return e
		}
		if !strings.EqualFold(m.ArchiveSHA256, l.PHP.ArchiveSHA256) {
			return fmt.Errorf("installed archive checksum differs from phpvm.lock")
		}
		fmt.Fprintln(a.Out, "Frozen environment matches", l.PHP.ID())
		return nil
	}
	o := defaultOptions()
	o.variant = l.PHP.Variant
	o.arch = l.PHP.Arch
	o.allowUnverifiedArchive = allow
	id, err := a.install(ctx, s, l.PHP.Version, o)
	if err != nil {
		return err
	}
	if err = s.Use(id); err != nil {
		return err
	}
	dir := filepath.Dir(s.Executable(id))
	iniPath, err := ensureINI(dir)
	if err != nil {
		return err
	}
	for k, v := range l.INI {
		if err = setINI(iniPath, k, v); err != nil {
			return err
		}
	}
	for _, name := range l.Extensions {
		_ = toggleExtension(iniPath, name, true)
	}
	fmt.Fprintln(a.Out, "Restored", id)
	return nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (a *App) composer(ctx context.Context, s *store.Store, args []string) error {
	if len(args) == 1 && (args[0] == "setup" || args[0] == "self-update") {
		return a.installComposer(ctx, s)
	}
	path := filepath.Join(s.Root, "tools", "composer.phar")
	if len(args) == 1 && args[0] == "path" {
		fmt.Fprintln(a.Out, path)
		return nil
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("Composer is not installed; run phpvm composer setup")
	}
	sep := -1
	for i, arg := range args {
		if arg == "--" {
			sep = i
			break
		}
	}
	if sep < 0 || sep == len(args)-1 {
		return fmt.Errorf("usage: phpvm composer setup|self-update|path|[version] -- <composer-args...>")
	}
	id := ""
	var err error
	if sep == 0 {
		id, err = resolveRuntimeBuild(s, "")
	} else if sep == 1 {
		id, err = a.install(ctx, s, args[0], defaultOptions())
	} else {
		return fmt.Errorf("only one PHP version may precede --")
	}
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, s.Executable(id), append([]string{path}, args[sep+1:]...)...)
	cmd.Env = append(os.Environ(), "COMPOSER_HOME="+filepath.Join(s.Root, "composer"), "COMPOSER_CACHE_DIR="+filepath.Join(s.Root, "cache", "composer"))
	cmd.Stdin = os.Stdin
	cmd.Stdout = a.Out
	cmd.Stderr = a.Err
	return cmd.Run()
}

func (a *App) installComposer(ctx context.Context, s *store.Store) error {
	const base = "https://getcomposer.org/download/latest-stable/composer.phar"
	b, err := download(ctx, base)
	if err != nil {
		return err
	}
	sum, err := download(ctx, base+".sha256")
	if err != nil {
		return err
	}
	got := sha256.Sum256(b)
	if !strings.EqualFold(hex.EncodeToString(got[:]), strings.TrimSpace(string(sum))) {
		return fmt.Errorf("Composer checksum mismatch")
	}
	path := filepath.Join(s.Root, "tools", "composer.phar")
	if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if err = os.WriteFile(path, b, 0644); err != nil {
		return err
	}
	fmt.Fprintln(a.Out, "Installed Composer", path)
	return nil
}

func (a *App) importBuild(s *store.Store, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: phpvm import <directory> [--name version] [--ts] [--arch x64|x86]")
	}
	source, err := filepath.Abs(args[0])
	if err != nil {
		return err
	}
	variant := "nts"
	arch := "x64"
	name := ""
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--ts":
			variant = "ts"
		case "--arch", "--name":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value", args[i])
			}
			i++
			if args[i-1] == "--arch" {
				arch = args[i]
			} else {
				name = args[i]
			}
		default:
			return fmt.Errorf("unknown option %s", args[i])
		}
	}
	php := filepath.Join(source, "php.exe")
	if _, err = os.Stat(php); err != nil {
		return fmt.Errorf("%s does not contain php.exe", source)
	}
	if name == "" {
		out, e := exec.Command(php, "-r", "echo PHP_VERSION;").Output()
		if e != nil {
			return fmt.Errorf("inspect imported PHP: %w", e)
		}
		name = strings.TrimSpace(string(out))
	}
	m := store.Metadata{Version: name, Variant: variant, Arch: arch, URL: "file://" + filepath.ToSlash(source)}
	if err = s.Import(source, m); err != nil {
		return err
	}
	fmt.Fprintln(a.Out, "Imported", m.ID())
	return nil
}

func download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func extractExtensionZIP(payload []byte, dir string) ([]string, error) {
	tmp, err := os.CreateTemp("", "phpvm-ext-*.zip")
	if err != nil {
		return nil, err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.Write(payload); err != nil {
		return nil, err
	}
	if err = tmp.Close(); err != nil {
		return nil, err
	}
	r, err := zip.OpenReader(name)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	var installed []string
	for _, f := range r.File {
		base := filepath.Base(f.Name)
		if f.FileInfo().IsDir() || !strings.HasSuffix(strings.ToLower(base), ".dll") {
			continue
		}
		src, e := f.Open()
		if e != nil {
			return nil, e
		}
		target := filepath.Join(dir, base)
		dst, e := os.Create(target)
		if e != nil {
			src.Close()
			return nil, e
		}
		_, e = io.Copy(dst, src)
		src.Close()
		dst.Close()
		if e != nil {
			return nil, e
		}
		installed = append(installed, base)
	}
	if len(installed) == 0 {
		return nil, fmt.Errorf("package contains no DLL extension")
	}
	return installed, nil
}
