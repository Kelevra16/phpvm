package app

import (
	"bufio"
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
	"time"

	"github.com/Kelevra16/phpvm/internal/store"
	"github.com/Kelevra16/phpvm/internal/windowsphp"
)

type App struct {
	Version  string
	In       io.Reader
	Out, Err io.Writer
	ui       *console
	input    *bufio.Reader
}

func New(version string) *App {
	return &App{Version: version, In: os.Stdin, Out: os.Stdout, Err: os.Stderr}
}

type buildOptions struct {
	variant, arch                                                                string
	json, quiet, noProgress, all, supportedOnly, allowUnverifiedArchive, offline bool
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
	fs.BoolVar(&o.supportedOnly, "supported-only", false, "")
	fs.BoolVar(&o.allowUnverifiedArchive, "allow-unverified-archive", false, "")
	fs.BoolVar(&o.offline, "offline", false, "")
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
	if o.json {
		o.quiet = true
		o.noProgress = true
	}
	return o, fs.Args(), nil
}

func (a *App) Run(ctx context.Context, args []string) error {
	args, plain, verbose := stripGlobalFlags(args)
	a.ui = newConsole(a.Out, a.Err, plain, verbose)
	root, err := rootDir()
	if err != nil {
		return err
	}
	s := store.New(root)
	a.ui.Debug("root: %s", root)
	if len(args) == 0 {
		return a.smart(ctx, s)
	}
	if err := guardRiskyCommand(s, args); err != nil {
		return err
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
	case "info":
		return a.info(ctx, s, args[1:])
	case "status":
		return a.status(s, args[1:])
	case "dashboard":
		return a.dashboard(s, args[1:])
	case "ui":
		return a.interactiveUI(ctx, s, args[1:])
	case "init":
		return a.initProject(s, args[1:])
	case "open":
		return a.openTarget(s, args[1:])
	case "trust":
		return a.trust(s, args[1:])
	case "check":
		return a.checkProject(s, args[1:])
	case "serve":
		return a.serve(ctx, s, args[1:])
	case "pie":
		return a.pie(ctx, s, args[1:])
	case "supported":
		return a.supported(ctx, s, args[1:])
	case "runtime":
		return a.runtimeInfo(args[1:])
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
	case "bundle":
		return a.bundle(s, args[1:])
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
		if len(args) == 2 && args[1] == "--fix" {
			return a.doctorFix(s)
		}
		if len(args) == 2 && args[1] == "--interactive" {
			return a.interactiveDoctor(s)
		}
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
	case "lock":
		return a.lockProject(s, args[1:])
	case "restore":
		return a.restoreProject(ctx, s, args[1:])
	case "composer":
		return a.composer(ctx, s, args[1:])
	case "import":
		return a.importBuild(s, args[1:])
	case "matrix":
		return a.matrix(ctx, s, args[1:])
	case "uninstall", "remove", "rm":
		uninstallArgs, yes := withoutYes(args[1:])
		if len(uninstallArgs) != 1 {
			return fmt.Errorf("usage: phpvm uninstall [--yes] <build>")
		}
		id, err := resolveInstalled(s, uninstallArgs[0])
		if err != nil {
			return err
		}
		if a.canPrompt() && !yes {
			confirmed, confirmErr := a.confirm(tr("Remove "+id+"?", "¿Eliminar "+id+"?"), false)
			if confirmErr != nil {
				return confirmErr
			}
			if !confirmed {
				a.ui.Info(tr("Cancelled", "Cancelado"))
				return nil
			}
		}
		if err := s.Uninstall(id); err != nil {
			return err
		}
		fmt.Fprintln(a.Out, "Removed", id)
		return nil
	case "prune":
		pruneArgs, yes := withoutYes(args[1:])
		if len(pruneArgs) != 0 {
			return fmt.Errorf("usage: phpvm prune [--yes]")
		}
		if a.canPrompt() && !yes {
			confirmed, confirmErr := a.confirm(tr("Remove every build except the active one?", "¿Eliminar todas las versiones excepto la activa?"), false)
			if confirmErr != nil {
				return confirmErr
			}
			if !confirmed {
				a.ui.Info(tr("Cancelled", "Cancelado"))
				return nil
			}
		}
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
	if len(rest) == 0 && a.canPrompt() {
		var selected string
		if command == "use" {
			selected, err = a.chooseInstalled(s)
		} else {
			selected, err = a.chooseRemote(ctx, s, o)
		}
		if err != nil {
			return err
		}
		rest = []string{selected}
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
			a.ui.Success("Using PHP %s", id)
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
	// Imported and custom builds may not exist in the official registry.
	if s.IsInstalled(requested) {
		if !o.quiet {
			a.ui.Info("PHP %s is already installed", requested)
		}
		return requested, nil
	}
	p, err := provider(s.Root)
	if err != nil {
		return "", err
	}
	p.SetOffline(o.offline)
	rel, err := p.Resolve(ctx, requested, o.variant, o.arch)
	if err != nil {
		return "", err
	}
	a.ui.Debug("resolved %s to %s (%s/%s)", requested, rel.Version, rel.Variant, rel.Arch)
	if rel.Archived && rel.SHA256 == "" && !o.allowUnverifiedArchive {
		return "", fmt.Errorf("PHP %s is in the official EOL archive, which does not publish SHA-256 checksums; review the risk and retry with --allow-unverified-archive", rel.Version)
	}
	if windowsphp.IsEOL(rel.Version, time.Now()) && !o.quiet {
		a.ui.Warn("PHP %s is end-of-life and no longer receives security fixes.", rel.Version)
	}
	m := store.Metadata{Version: rel.Version, Variant: rel.Variant, Arch: rel.Arch, URL: rel.URL, ArchiveSHA256: rel.SHA256, Runtime: windowsphp.CompilerRuntime(rel.Version), SourceKind: "official"}
	if s.IsInstalled(m.ID()) {
		if !o.quiet {
			a.ui.Info("PHP %s is already installed", m.ID())
		}
		return m.ID(), nil
	}
	if !o.quiet {
		a.ui.Info("Installing PHP %s", m.ID())
	}
	if !o.quiet && !o.noProgress {
		s.Progress = func(done, total int64) {
			a.ui.Progress(done, total)
		}
		defer func() { s.Progress = nil }()
	}
	if !o.quiet {
		s.Stage = func(stage string) { a.ui.Step("%s", translatedStage(stage)) }
		defer func() { s.Stage = nil }()
	}
	previousOffline := s.Offline
	s.Offline = o.offline
	defer func() { s.Offline = previousOffline }()
	if err := s.Install(ctx, m); err != nil {
		return "", err
	}
	if !o.quiet {
		a.ui.Success("PHP %s installed", m.ID())
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
	rows := make([][]string, 0, len(builds))
	for _, m := range builds {
		mark := ""
		if m.ID() == current {
			mark = a.ui.symbol("●", "*")
		}
		rows = append(rows, []string{mark, m.Version, m.Variant, m.Arch, m.Runtime})
	}
	a.ui.Table([]string{"", "VERSION", "TYPE", "ARCH", "RUNTIME"}, rows)
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
	p.SetOffline(o.offline)
	var v []windowsphp.Release
	if o.all {
		v, err = p.AllVersions(ctx, o.variant, o.arch)
	} else {
		v, err = p.Versions(ctx, o.variant, o.arch)
	}
	if err != nil {
		return err
	}
	if o.supportedOnly {
		filtered := v[:0]
		for _, r := range v {
			if !windowsphp.IsEOL(r.Version, time.Now()) {
				filtered = append(filtered, r)
			}
		}
		v = filtered
	}
	if o.json {
		return json.NewEncoder(a.Out).Encode(v)
	}
	rows := make([][]string, 0, len(v))
	for _, r := range v {
		status := "supported"
		if windowsphp.IsEOL(r.Version, time.Now()) {
			status = "EOL"
		}
		rows = append(rows, []string{r.Version, r.Variant, r.Arch, status})
	}
	a.ui.Table([]string{"VERSION", "TYPE", "ARCH", "STATUS"}, rows)
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
	a.ui.Success("%s verified", id)
	return nil
}
func (a *App) repair(ctx context.Context, s *store.Store, args []string) error {
	clean, yes := withoutYes(args)
	id, err := targetBuild(s, clean)
	if err != nil {
		return err
	}
	if a.canPrompt() && !yes {
		confirmed, confirmErr := a.confirm(tr("Replace "+id+" from its recorded source?", "¿Reemplazar "+id+" desde su origen registrado?"), false)
		if confirmErr != nil {
			return confirmErr
		}
		if !confirmed {
			a.ui.Info(tr("Cancelled", "Cancelado"))
			return nil
		}
	}
	if err := s.Repair(ctx, id); err != nil {
		return err
	}
	a.ui.Success("Repaired %s", id)
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
		if m, metaErr := s.Metadata(id); metaErr == nil {
			runtimeName := windowsphp.CompilerRuntime(m.Version)
			out, runtimeErr := exec.Command(s.Executable(id), "--version").CombinedOutput()
			detail := "runtime available"
			if runtimeErr != nil {
				detail = strings.TrimSpace(string(out))
				if detail == "" {
					detail = runtimeErr.Error()
				}
			}
			checks = append(checks, check{"Visual C++ " + runtimeName, runtimeErr == nil, detail})
		}
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
		mark := a.ui.paint(ansiGreen, a.ui.symbol("✓", "OK"))
		if !c.OK {
			mark = a.ui.paint(ansiRed, a.ui.symbol("×", "FAIL"))
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
  phpvm [--plain|--no-color] [--verbose] <command>
  phpvm use [--ts] [--arch x64|x86] [--offline] [--allow-unverified-archive] <version>
  phpvm install [--ts] [--arch x64|x86] [--offline] [--allow-unverified-archive] <version>
  phpvm info [--json] <version>     phpvm supported [--json]
  phpvm ls [--json]                 phpvm ls-remote [--ts] [--all] [--json]
  phpvm current [--json]            phpvm verify [build]
  phpvm status [--json]             phpvm check [--json]
  phpvm dashboard                   phpvm ui
  phpvm init [--version V] [--preset P]
  phpvm open <logs|ini|root>
  phpvm which [build]               phpvm cache <dir|list|verify|clear>
  phpvm bundle <create|import> <file.zip>
  phpvm resolve [--path] [version]  phpvm shell [version|--current]
  phpvm self-update                 phpvm completion powershell
  phpvm repair [--yes] [build]      phpvm doctor [--json|--fix|--interactive]
  phpvm exec [version] -- <command> phpvm matrix <versions...> -- <command>
  phpvm alias [ls|set|remove]        phpvm sync
  phpvm lock | restore               phpvm composer [install|args...]
  phpvm trust <project|status|revoke> phpvm serve [--port 8080]
  phpvm pie <setup|path|args...>      phpvm import <directory>
  phpvm ini [target] <get|set>       phpvm profile <ls|create|set|use>
  phpvm ini [target] preset <name>
  phpvm ext [target] <ls|enable|disable|search|install|update>
  phpvm logs <path|show|tail|open|clear|doctor>
  phpvm laragon <detect|link|unlink>
  phpvm uninstall [--yes] <build>    phpvm prune [--yes] | clean
`)
}
