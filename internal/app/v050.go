package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Kelevra16/phpvm/internal/store"
	"github.com/Kelevra16/phpvm/internal/windowsphp"
)

func (a *App) status(s *store.Store, args []string) error {
	jsonOut := len(args) == 1 && args[0] == "--json"
	if len(args) > 1 || (len(args) == 1 && !jsonOut) {
		return fmt.Errorf("usage: phpvm status [--json]")
	}
	global, _ := s.Current()
	effective, effectiveErr := resolveRuntimeBuild(s, "")
	cfg, _ := findProjectConfig()
	ini := ""
	extCount := 0
	if effectiveErr == nil {
		dir := filepath.Dir(s.Executable(effective))
		ini = filepath.Join(dir, "php.ini")
		if enabled, e := enabledExtensions(ini); e == nil {
			extCount = len(enabled)
		}
	}
	trusted, _ := projectTrusted(s)
	data := map[string]any{"project": filepath.Base(currentDir()), "effective": effective, "global": global, "source": cfg.Source, "phpIni": ini, "extensions": extCount, "trusted": trusted, "safeMode": safeMode()}
	if jsonOut {
		return json.NewEncoder(a.Out).Encode(data)
	}
	rows := [][]string{{"Project", fmt.Sprint(data["project"])}, {"PHP effective", effective}, {"PHP global", global}, {"Configuration", cfg.Source}, {"php.ini", ini}, {"Extensions", fmt.Sprint(extCount)}, {"Trusted", fmt.Sprint(trusted)}, {"Safe mode", fmt.Sprint(safeMode())}}
	a.ui.Table([]string{"PROPERTY", "VALUE"}, rows)
	return effectiveErr
}

func currentDir() string { d, _ := os.Getwd(); return d }
func safeMode() bool {
	v := strings.ToLower(os.Getenv("PHPVM_SAFE_MODE"))
	return v == "1" || v == "true" || v == "on"
}

func guardRiskyCommand(s *store.Store, args []string) error {
	if len(args) == 0 {
		return nil
	}
	risky := false
	switch args[0] {
	case "exec", "shell", "import", "serve", "restore", "sync":
		risky = true
	case "composer":
		risky = len(args) < 2 || args[1] != "path"
	case "pie":
		risky = len(args) < 2 || args[1] != "path"
	case "ext":
		for _, v := range args[1:] {
			if v == "install" || v == "update" {
				risky = true
			}
		}
	}
	if !risky {
		return nil
	}
	if safeMode() {
		return fmt.Errorf("%s is disabled by PHPVM_SAFE_MODE", args[0])
	}
	composerExec := args[0] == "composer" && len(args) > 1 && args[1] != "setup" && args[1] != "self-update"
	pieExec := args[0] == "pie" && len(args) > 1 && args[1] != "setup"
	if args[0] == "serve" || args[0] == "restore" || args[0] == "sync" || composerExec || pieExec || (args[0] == "ext" && risky) {
		return requireProjectTrust(s, args[0])
	}
	return nil
}

type trustDB map[string]string

func trustPath(s *store.Store) string { return filepath.Join(s.Root, "trust.json") }
func projectFingerprint() (string, string, error) {
	root, err := filepath.Abs(currentDir())
	if err != nil {
		return "", "", err
	}
	names := []string{".php-version", "phpvm.toml", "phpvm.lock", "composer.json", "composer.lock"}
	for {
		foundHere := false
		for _, name := range names {
			if st, e := os.Stat(filepath.Join(root, name)); e == nil && !st.IsDir() {
				foundHere = true
				break
			}
		}
		if foundHere {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			return root, "", fmt.Errorf("no project configuration found")
		}
		root = parent
	}
	h := sha256.New()
	found := false
	for _, name := range names {
		b, e := os.ReadFile(filepath.Join(root, name))
		if e == nil {
			found = true
			_, _ = h.Write([]byte(name + "\x00"))
			_, _ = h.Write(b)
		}
	}
	if !found {
		return root, "", fmt.Errorf("no project configuration found")
	}
	return root, hex.EncodeToString(h.Sum(nil)), nil
}
func loadTrust(s *store.Store) (trustDB, error) {
	db := trustDB{}
	b, e := os.ReadFile(trustPath(s))
	if os.IsNotExist(e) {
		return db, nil
	}
	if e != nil {
		return nil, e
	}
	return db, json.Unmarshal(b, &db)
}
func projectTrusted(s *store.Store) (bool, error) {
	root, sum, e := projectFingerprint()
	if e != nil {
		return false, nil
	}
	db, e := loadTrust(s)
	if e != nil {
		return false, e
	}
	return strings.EqualFold(db[root], sum), nil
}
func requireProjectTrust(s *store.Store, action string) error {
	if safeMode() {
		return fmt.Errorf("%s is disabled by PHPVM_SAFE_MODE", action)
	}
	ok, e := projectTrusted(s)
	if e != nil {
		return e
	}
	if !ok {
		return fmt.Errorf("project is not trusted; review its files and run phpvm trust project before %s", action)
	}
	return nil
}
func (a *App) trust(s *store.Store, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: phpvm trust <project|status|revoke>")
	}
	root, sum, e := projectFingerprint()
	if e != nil {
		return e
	}
	db, e := loadTrust(s)
	if e != nil {
		return e
	}
	switch args[0] {
	case "project":
		db[root] = sum
		if e = writeJSON(trustPath(s), db); e == nil {
			fmt.Fprintln(a.Out, "Trusted", root)
		}
		return e
	case "status":
		ok := strings.EqualFold(db[root], sum)
		fmt.Fprintln(a.Out, map[bool]string{true: "trusted", false: "untrusted"}[ok], root)
		if !ok {
			return fmt.Errorf("project trust is missing or stale")
		}
		return nil
	case "revoke":
		delete(db, root)
		if e = writeJSON(trustPath(s), db); e == nil {
			fmt.Fprintln(a.Out, "Revoked trust for", root)
		}
		return e
	default:
		return fmt.Errorf("usage: phpvm trust <project|status|revoke>")
	}
}

func (a *App) applyINIPreset(id, dir, name string) error {
	allowed := map[string]map[string]string{"development": {"display_errors": "On", "display_startup_errors": "On", "error_reporting": "E_ALL", "memory_limit": "512M"}, "production": {"display_errors": "Off", "display_startup_errors": "Off", "error_reporting": "E_ALL & ~E_DEPRECATED & ~E_STRICT", "memory_limit": "256M", "opcache.enable": "1"}, "testing": {"display_errors": "On", "error_reporting": "E_ALL", "memory_limit": "1G", "max_execution_time": "0"}, "codeigniter": {"display_errors": "On", "memory_limit": "512M"}, "laravel": {"display_errors": "On", "memory_limit": "512M"}, "wordpress": {"display_errors": "Off", "memory_limit": "256M"}}
	settings, ok := allowed[strings.ToLower(name)]
	if !ok {
		return fmt.Errorf("unknown preset %s; use development, production, testing, codeigniter, laravel, or wordpress", name)
	}
	extensions, e := store.ConfigureDefaults(dir)
	if e != nil {
		return e
	}
	ini := filepath.Join(dir, "php.ini")
	for k, v := range settings {
		if e = setINI(ini, k, v); e != nil {
			return e
		}
	}
	fmt.Fprintf(a.Out, "Applied %s preset to %s (%d extensions)\n", name, id, len(extensions))
	return nil
}

func (a *App) doctorFix(s *store.Store) error {
	id, e := s.Current()
	if e != nil {
		return e
	}
	dir := filepath.Dir(s.Executable(id))
	if _, e = ensureINI(dir); e != nil {
		return e
	}
	if e = s.Use(id); e != nil {
		return e
	}
	enabled, _ := enabledExtensions(filepath.Join(dir, "php.ini"))
	for name := range enabled {
		_ = toggleExtension(filepath.Join(dir, "php.ini"), name, true)
	}
	fmt.Fprintln(a.Out, "Applied safe repairs to", id)
	return a.doctor(s, nil)
}

func (a *App) checkProject(s *store.Store, args []string) error {
	jsonOut := len(args) == 1 && args[0] == "--json"
	if len(args) > 1 || (len(args) == 1 && !jsonOut) {
		return fmt.Errorf("usage: phpvm check [--json]")
	}
	b, e := os.ReadFile("composer.json")
	if e != nil {
		return e
	}
	var doc struct {
		Require    map[string]string `json:"require"`
		RequireDev map[string]string `json:"require-dev"`
	}
	if e = json.Unmarshal(b, &doc); e != nil {
		return e
	}
	id, e := resolveRuntimeBuild(s, "")
	if e != nil {
		return e
	}
	m, e := s.Metadata(id)
	if e != nil {
		return e
	}
	loaded := map[string]bool{}
	if out, runErr := exec.Command(s.Executable(id), "-r", "echo json_encode(get_loaded_extensions());").Output(); runErr == nil {
		var names []string
		if json.Unmarshal(out, &names) == nil {
			for _, name := range names {
				loaded[strings.ToLower(name)] = true
			}
		}
	}
	type result struct {
		Name, Required, Actual string
		OK                     bool
	}
	var results []result
	all := map[string]string{}
	for k, v := range doc.Require {
		all[k] = v
	}
	for k, v := range doc.RequireDev {
		if _, ok := all[k]; !ok {
			all[k] = v
		}
	}
	for k, v := range all {
		if k == "php" {
			results = append(results, result{k, v, m.Version, windowsphp.Satisfies(m.Version, v)})
			continue
		}
		if strings.HasPrefix(k, "ext-") {
			name := strings.TrimPrefix(k, "ext-")
			ok := loaded[strings.ToLower(name)]
			results = append(results, result{k, v, map[bool]string{true: "loaded", false: "missing"}[ok], ok})
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Name < results[j].Name })
	if jsonOut {
		return json.NewEncoder(a.Out).Encode(results)
	}
	failed := false
	rows := [][]string{}
	for _, r := range results {
		state := "OK"
		if !r.OK {
			state = "FAIL"
			failed = true
		}
		rows = append(rows, []string{state, r.Name, r.Required, r.Actual})
	}
	a.ui.Table([]string{"STATUS", "REQUIREMENT", "REQUIRED", "ACTUAL"}, rows)
	if failed {
		return fmt.Errorf("project platform requirements are not satisfied")
	}
	return nil
}

func (a *App) serve(ctx context.Context, s *store.Store, args []string) error {
	if e := requireProjectTrust(s, "starting the development server"); e != nil {
		return e
	}
	port, public, router := "8080", "public", ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--port", "--public", "--router":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value", args[i])
			}
			key := args[i]
			i++
			if key == "--port" {
				port = args[i]
			} else if key == "--public" {
				public = args[i]
			} else {
				router = args[i]
			}
		default:
			return fmt.Errorf("unknown serve option %s", args[i])
		}
	}
	root := currentDir()
	portNumber, e := strconv.Atoi(port)
	if e != nil || portNumber < 1 || portNumber > 65535 {
		return fmt.Errorf("port must be an integer between 1 and 65535")
	}
	docroot, e := containedPath(root, public)
	if e != nil {
		return e
	}
	if st, statErr := os.Stat(docroot); statErr != nil || !st.IsDir() {
		return fmt.Errorf("public directory does not exist: %s", docroot)
	}
	id, e := resolveRuntimeBuild(s, "")
	if e != nil {
		return e
	}
	cmdArgs := []string{"-S", "127.0.0.1:" + port, "-t", docroot}
	if router != "" {
		p, e := containedPath(root, router)
		if e != nil {
			return e
		}
		if st, statErr := os.Stat(p); statErr != nil || st.IsDir() {
			return fmt.Errorf("router file does not exist: %s", p)
		}
		cmdArgs = append(cmdArgs, p)
	}
	fmt.Fprintf(a.Out, "Serving %s with %s at http://127.0.0.1:%s\n", docroot, id, port)
	cmd := exec.CommandContext(ctx, s.Executable(id), cmdArgs...)
	cmd.Dir = root
	cmd.Stdin = os.Stdin
	cmd.Stdout = a.Out
	cmd.Stderr = a.Err
	return cmd.Run()
}
func containedPath(root, value string) (string, error) {
	p := value
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	p, e := filepath.Abs(p)
	if e != nil {
		return "", e
	}
	rel, e := filepath.Rel(root, p)
	if e != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path %s escapes project root", value)
	}
	return p, nil
}

func (a *App) pie(ctx context.Context, s *store.Store, args []string) error {
	path := filepath.Join(s.Root, "tools", "pie.phar")
	if len(args) == 1 && args[0] == "path" {
		fmt.Fprintln(a.Out, path)
		return nil
	}
	if len(args) == 1 && args[0] == "setup" {
		if safeMode() {
			return fmt.Errorf("PIE setup is disabled by PHPVM_SAFE_MODE")
		}
		if _, e := exec.LookPath("gh"); e != nil {
			return fmt.Errorf("PIE setup requires GitHub CLI for attestation verification")
		}
		b, e := download(ctx, "https://github.com/php/pie/releases/latest/download/pie.phar")
		if e != nil {
			return e
		}
		if e = os.MkdirAll(filepath.Dir(path), 0755); e != nil {
			return e
		}
		stage := path + ".new"
		if e = os.WriteFile(stage, b, 0644); e != nil {
			return e
		}
		defer os.Remove(stage)
		cmd := exec.CommandContext(ctx, "gh", "attestation", "verify", stage, "--repo", "php/pie")
		if out, e := cmd.CombinedOutput(); e != nil {
			return fmt.Errorf("PIE attestation verification failed: %s", strings.TrimSpace(string(out)))
		}
		if e = os.Rename(stage, path); e != nil {
			return e
		}
		fmt.Fprintln(a.Out, "Installed verified PIE", path)
		return nil
	}
	if e := requireProjectTrust(s, "running PIE"); e != nil {
		return e
	}
	if _, e := os.Stat(path); e != nil {
		return fmt.Errorf("PIE is not installed; run phpvm pie setup")
	}
	id, e := resolveRuntimeBuild(s, "")
	if e != nil {
		return e
	}
	runID, e := resolveRuntimeBuild(s, ">=8.1")
	if e != nil {
		return fmt.Errorf("PIE requires an installed PHP 8.1 or newer")
	}
	pieArgs := append([]string{path}, args...)
	pieArgs = append(pieArgs, "--with-php-path="+s.Executable(id))
	cmd := exec.CommandContext(ctx, s.Executable(runID), pieArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = a.Out
	cmd.Stderr = a.Err
	return cmd.Run()
}
