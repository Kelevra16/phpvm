package app

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Kelevra16/phpvm/internal/store"
	"github.com/Kelevra16/phpvm/internal/windowsphp"
)

type App struct {
	Version  string
	Out, Err io.Writer
}

func New(version string) *App { return &App{Version: version, Out: os.Stdout, Err: os.Stderr} }

type buildOptions struct {
	variant, arch                                        string
	json, quiet, noProgress, all, allowUnverifiedArchive bool
}

func defaultOptions() buildOptions {
	arch := "x64"
	if runtime.GOARCH == "386" {
		arch = "x86"
	}
	return buildOptions{variant: "nts", arch: arch}
}
func parseBuildFlags(name string, args []string) (buildOptions, []string, error) {
	o := defaultOptions()
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	ts := fs.Bool("ts", false, "")
	fs.BoolVar(&o.json, "json", false, "")
	fs.BoolVar(&o.quiet, "quiet", false, "")
	fs.BoolVar(&o.noProgress, "no-progress", false, "")
	fs.BoolVar(&o.all, "all", false, "")
	fs.BoolVar(&o.allowUnverifiedArchive, "allow-unverified-archive", false, "")
	fs.StringVar(&o.arch, "arch", o.arch, "")
	if err := fs.Parse(args); err != nil {
		return o, nil, err
	}
	if *ts {
		o.variant = "ts"
	}
	if o.arch != "x64" && o.arch != "x86" {
		return o, nil, fmt.Errorf("arch must be x64 or x86")
	}
	return o, fs.Args(), nil
}

func (a *App) Run(ctx context.Context, args []string) error {
	root, err := rootDir()
	if err != nil {
		return err
	}
	s := store.New(root)
	if len(args) == 0 {
		return a.smart(ctx, s)
	}
	switch args[0] {
	case "help", "-h", "--help":
		a.help()
		return nil
	case "version", "--version":
		fmt.Fprintln(a.Out, "phpvm", a.Version)
		return nil
	case "install", "use":
		return a.installCommand(ctx, s, args[0], args[1:])
	case "list", "ls":
		return a.list(s, args[1:])
	case "ls-remote":
		return a.remote(ctx, args[1:])
	case "current":
		return a.current(s, args[1:])
	case "which":
		return a.which(s, args[1:])
	case "resolve":
		return a.resolve(s, args[1:])
	case "shell":
		return a.shell(ctx, s, args[1:])
	case "cache":
		return a.cache(s, args[1:])
	case "self-update":
		return a.selfUpdate(ctx, args[1:])
	case "completion":
		return a.completion(args[1:])
	case "laragon":
		return a.laragon(s, args[1:])
	case "verify":
		return a.verify(s, args[1:])
	case "repair":
		return a.repair(ctx, s, args[1:])
	case "doctor":
		return a.doctor(s, args[1:])
	case "clean":
		removed, err := s.Clean()
		if err != nil {
			return err
		}
		for _, p := range removed {
			fmt.Fprintln(a.Out, "Removed", p)
		}
		return nil
	case "exec":
		return a.execute(ctx, s, args[1:])
	case "alias":
		return a.alias(root, args[1:])
	case "ini":
		return a.ini(s, args[1:])
	case "profile":
		return a.profile(s, args[1:])
	case "ext":
		return a.extensions(s, args[1:])
	case "logs", "log":
		return a.logs(ctx, s, args[1:])
	case "sync":
		return a.sync(ctx, s)
	case "matrix":
		return a.matrix(ctx, s, args[1:])
	case "uninstall", "remove", "rm":
		if len(args) != 2 {
			return fmt.Errorf("usage: phpvm uninstall <build>")
		}
		id, err := resolveInstalled(s, args[1])
		if err != nil {
			return err
		}
		if err := s.Uninstall(id); err != nil {
			return err
		}
		fmt.Fprintln(a.Out, "Removed", id)
		return nil
	case "prune":
		removed, err := s.Prune()
		if err != nil {
			return err
		}
		for _, id := range removed {
			fmt.Fprintln(a.Out, "Removed", id)
		}
		return nil
	default:
		return fmt.Errorf("unknown command %q (run phpvm help)", args[0])
	}
}

func (a *App) installCommand(ctx context.Context, s *store.Store, command string, args []string) error {
	o, rest, err := parseBuildFlags(command, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return fmt.Errorf("usage: phpvm %s [--ts] [--arch x64|x86] [--allow-unverified-archive] <version>", command)
	}
	id, err := a.install(ctx, s, rest[0], o)
	if err != nil {
		return err
	}
	if command == "use" {
		if err := s.Use(id); err != nil {
			return err
		}
		if !o.quiet {
			fmt.Fprintln(a.Out, "Using PHP", id)
		}
	}
	if o.json {
		return json.NewEncoder(a.Out).Encode(map[string]string{"build": id})
	}
	return nil
}
func (a *App) install(ctx context.Context, s *store.Store, requested string, o buildOptions) (string, error) {
	if alias, ok, _ := readAliases(s.Root); ok {
		if v, found := alias[requested]; found {
			requested = v
		}
	}
	if requested == "auto" {
		cfg, err := findProjectConfig()
		if err != nil {
			return "", err
		}
		if cfg.Version == "" {
			return "", fmt.Errorf("no project PHP version found")
		}
		requested = cfg.Version
		if cfg.Variant != "" {
			o.variant = cfg.Variant
		}
		if cfg.Arch != "" {
			o.arch = cfg.Arch
		}
	}
	p, err := provider(s.Root)
	if err != nil {
		return "", err
	}
	rel, err := p.Resolve(ctx, requested, o.variant, o.arch)
	if err != nil {
		return "", err
	}
	if rel.Archived && rel.SHA256 == "" && !o.allowUnverifiedArchive {
		return "", fmt.Errorf("PHP %s is in the official EOL archive, which does not publish SHA-256 checksums; review the risk and retry with --allow-unverified-archive", rel.Version)
	}
	m := store.Metadata{Version: rel.Version, Variant: rel.Variant, Arch: rel.Arch, URL: rel.URL, ArchiveSHA256: rel.SHA256}
	if s.IsInstalled(m.ID()) {
		if !o.quiet {
			fmt.Fprintln(a.Out, "PHP", m.ID(), "is already installed")
		}
		return m.ID(), nil
	}
	if !o.quiet {
		fmt.Fprintln(a.Out, "Installing PHP", m.ID()+"...")
	}
	if !o.quiet && !o.noProgress {
		last := -1
		s.Progress = func(done, total int64) {
			if total <= 0 {
				return
			}
			percent := int(done * 100 / total)
			if percent/10 != last/10 {
				fmt.Fprintf(a.Out, "Downloading %d%%\n", percent)
				last = percent
			}
		}
		defer func() { s.Progress = nil }()
	}
	if err := s.Install(ctx, m); err != nil {
		return "", err
	}
	if !o.quiet {
		fmt.Fprintln(a.Out, "Installed PHP", m.ID())
	}
	return m.ID(), nil
}
func (a *App) smart(ctx context.Context, s *store.Store) error {
	cfg, err := findProjectConfig()
	if err != nil {
		return err
	}
	if cfg.Version == "" {
		a.help()
		return nil
	}
	o := defaultOptions()
	if cfg.Variant != "" {
		o.variant = cfg.Variant
	}
	if cfg.Arch != "" {
		o.arch = cfg.Arch
	}
	id, err := a.install(ctx, s, cfg.Version, o)
	if err != nil {
		return err
	}
	if err := s.Use(id); err != nil {
		return err
	}
	fmt.Fprintln(a.Out, "Using PHP", id)
	return nil
}

func (a *App) list(s *store.Store, args []string) error {
	asJSON := len(args) == 1 && args[0] == "--json"
	builds, err := s.Installed()
	if err != nil {
		return err
	}
	current, _ := s.Current()
	if asJSON {
		return json.NewEncoder(a.Out).Encode(map[string]any{"current": current, "builds": builds})
	}
	for _, m := range builds {
		mark := "  "
		if m.ID() == current {
			mark = "* "
		}
		fmt.Fprintln(a.Out, mark+m.ID())
	}
	return nil
}
func (a *App) remote(ctx context.Context, args []string) error {
	o, rest, err := parseBuildFlags("ls-remote", args)
	if err != nil || len(rest) > 0 {
		if err != nil {
			return err
		}
		return fmt.Errorf("usage: phpvm ls-remote [--ts] [--arch x64|x86] [--all] [--json]")
	}
	root, _ := rootDir()
	p, err := provider(root)
	if err != nil {
		return err
	}
	var v []windowsphp.Release
	if o.all {
		v, err = p.AllVersions(ctx, o.variant, o.arch)
	} else {
		v, err = p.Versions(ctx, o.variant, o.arch)
	}
	if err != nil {
		return err
	}
	if o.json {
		return json.NewEncoder(a.Out).Encode(v)
	}
	for _, r := range v {
		fmt.Fprintln(a.Out, r.Version, r.Variant, r.Arch)
	}
	return nil
}
func (a *App) current(s *store.Store, args []string) error {
	id, err := s.Current()
	if err != nil {
		return err
	}
	m, err := s.Metadata(id)
	if err != nil {
		return err
	}
	if len(args) == 1 && args[0] == "--json" {
		return json.NewEncoder(a.Out).Encode(m)
	}
	fmt.Fprintln(a.Out, id)
	return nil
}
func (a *App) verify(s *store.Store, args []string) error {
	id, err := targetBuild(s, args)
	if err != nil {
		return err
	}
	if err := s.Verify(id); err != nil {
		return fmt.Errorf("%s: %w", id, err)
	}
	fmt.Fprintln(a.Out, id, "OK")
	return nil
}
func (a *App) repair(ctx context.Context, s *store.Store, args []string) error {
	id, err := targetBuild(s, args)
	if err != nil {
		return err
	}
	if err := s.Repair(ctx, id); err != nil {
		return err
	}
	fmt.Fprintln(a.Out, "Repaired", id)
	return nil
}
func targetBuild(s *store.Store, args []string) (string, error) {
	if len(args) > 1 {
		return "", fmt.Errorf("expected zero or one build")
	}
	if len(args) == 0 {
		return s.Current()
	}
	return resolveInstalled(s, args[0])
}
func resolveInstalled(s *store.Store, q string) (string, error) {
	if s.IsInstalled(q) {
		return q, nil
	}
	builds, err := s.Installed()
	if err != nil {
		return "", err
	}
	var matches []string
	for _, m := range builds {
		if m.Version == q || strings.HasPrefix(m.ID(), q+"-") {
			matches = append(matches, m.ID())
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("%s matches multiple builds: %s", q, strings.Join(matches, ", "))
	}
	return "", fmt.Errorf("PHP build %s is not installed", q)
}

func (a *App) doctor(s *store.Store, args []string) error {
	type check struct {
		Name   string `json:"name"`
		OK     bool   `json:"ok"`
		Detail string `json:"detail"`
	}
	var checks []check
	id, err := s.Current()
	checks = append(checks, check{"active build", err == nil, first(err, id)})
	if err == nil {
		e := s.Verify(id)
		checks = append(checks, check{"active executable", e == nil, first(e, "checksum valid")})
	}
	_, e := os.Stat(filepath.Join(s.Root, "bin", "php.cmd"))
	checks = append(checks, check{"PATH wrapper", e == nil, first(e, filepath.Join(s.Root, "bin", "php.cmd"))})
	found, e := exec.LookPath("php")
	expected := strings.HasPrefix(strings.ToLower(found), strings.ToLower(filepath.Join(s.Root, "bin")))
	checks = append(checks, check{"php resolution", e == nil && expected, first(e, found)})
	asJSON := len(args) == 1 && args[0] == "--json"
	if asJSON {
		return json.NewEncoder(a.Out).Encode(checks)
	}
	failed := false
	for _, c := range checks {
		mark := "OK"
		if !c.OK {
			mark = "FAIL"
			failed = true
		}
		fmt.Fprintf(a.Out, "%-4s %-20s %s\n", mark, c.Name, c.Detail)
	}
	if failed {
		return fmt.Errorf("doctor found problems")
	}
	return nil
}
func first(err error, ok string) string {
	if err != nil {
		return err.Error()
	}
	return ok
}

func (a *App) execute(ctx context.Context, s *store.Store, args []string) error {
	sep := -1
	for i, v := range args {
		if v == "--" {
			sep = i
			break
		}
	}
	if sep < 0 || sep == len(args)-1 {
		return fmt.Errorf("usage: phpvm exec [version] -- <command> [args...]")
	}
	id := ""
	var err error
	if sep == 0 {
		id, err = s.Current()
	} else {
		o := defaultOptions()
		id, err = a.install(ctx, s, args[0], o)
	}
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, args[sep+1], args[sep+2:]...)
	cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(s.Executable(id))+string(os.PathListSeparator)+os.Getenv("PATH"), "PHPVM_ACTIVE="+id)
	cmd.Stdin = os.Stdin
	cmd.Stdout = a.Out
	cmd.Stderr = a.Err
	return cmd.Run()
}

func (a *App) alias(root string, args []string) error {
	aliases, _, err := readAliases(root)
	if err != nil {
		return err
	}
	if len(args) == 0 || args[0] == "ls" {
		for k, v := range aliases {
			fmt.Fprintln(a.Out, k, v)
		}
		return nil
	}
	if len(args) == 3 && args[0] == "set" {
		aliases[args[1]] = args[2]
		return writeJSON(filepath.Join(root, "aliases.json"), aliases)
	}
	if len(args) == 2 && args[0] == "remove" {
		delete(aliases, args[1])
		return writeJSON(filepath.Join(root, "aliases.json"), aliases)
	}
	return fmt.Errorf("usage: phpvm alias [ls|set <name> <version>|remove <name>]")
}
func readAliases(root string) (map[string]string, bool, error) {
	m := map[string]string{}
	b, err := os.ReadFile(filepath.Join(root, "aliases.json"))
	if os.IsNotExist(err) {
		return m, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	return m, true, json.Unmarshal(b, &m)
}
func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0644)
}

func (a *App) sync(ctx context.Context, s *store.Store) error {
	cfg, err := findProjectConfig()
	if err != nil {
		return err
	}
	if cfg.Version == "" {
		return fmt.Errorf("no .php-version, phpvm.toml, composer.lock, or composer.json found")
	}
	o := defaultOptions()
	if cfg.Variant != "" {
		o.variant = cfg.Variant
	}
	if cfg.Arch != "" {
		o.arch = cfg.Arch
	}
	id, err := a.install(ctx, s, cfg.Version, o)
	if err != nil {
		return err
	}
	if err := s.Use(id); err != nil {
		return err
	}
	for k, v := range cfg.INI {
		if err := setINI(filepath.Join(filepath.Dir(s.Executable(id)), "php.ini"), k, v); err != nil {
			return err
		}
	}
	fmt.Fprintln(a.Out, "Synchronized", id)
	return nil
}
func (a *App) matrix(ctx context.Context, s *store.Store, args []string) error {
	sep := -1
	for i, v := range args {
		if v == "--" {
			sep = i
			break
		}
	}
	if sep < 1 || sep == len(args)-1 {
		return fmt.Errorf("usage: phpvm matrix <versions...> -- <command>")
	}
	failed := false
	for _, v := range args[:sep] {
		fmt.Fprintln(a.Out, "==> PHP", v)
		if err := a.execute(ctx, s, append([]string{v, "--"}, args[sep+1:]...)); err != nil {
			failed = true
			fmt.Fprintln(a.Err, v, "FAIL:", err)
		} else {
			fmt.Fprintln(a.Out, v, "PASS")
		}
	}
	if failed {
		return fmt.Errorf("one or more matrix jobs failed")
	}
	return nil
}

func provider(root string) (*windowsphp.Provider, error) {
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("binary installation currently supports Windows only")
	}
	return windowsphp.New(root), nil
}
func rootDir() (string, error) {
	if v := os.Getenv("PHPVM_ROOT"); v != "" {
		return filepath.Abs(v)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".phpvm"), nil
}

func (a *App) help() {
	fmt.Fprint(a.Out, `phpvm - PHP version and environment manager

Usage:
  phpvm use [--ts] [--arch x64|x86] [--allow-unverified-archive] <version>
  phpvm install [--ts] [--arch x64|x86] [--allow-unverified-archive] <version>
  phpvm ls [--json]                 phpvm ls-remote [--ts] [--all] [--json]
  phpvm current [--json]            phpvm verify [build]
  phpvm which [build]               phpvm cache <dir|clear>
  phpvm resolve [--path] [version]  phpvm shell [version|--current]
  phpvm self-update                 phpvm completion powershell
  phpvm repair [build]              phpvm doctor [--json]
  phpvm exec [version] -- <command> phpvm matrix <versions...> -- <command>
  phpvm alias [ls|set|remove]        phpvm sync
  phpvm ini <get|set>                phpvm profile <ls|create|set|use>
  phpvm ext <ls|enable|disable>       phpvm logs <path|show|tail|open|clear|doctor>
  phpvm laragon <detect|link|unlink>
  phpvm uninstall <build>            phpvm prune | clean
`)
}
