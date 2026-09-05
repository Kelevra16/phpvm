package app

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Kelevra16/phpvm/internal/store"
	"github.com/Kelevra16/phpvm/internal/windowsphp"
)

func language() string {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("PHPVM_LANG")))
	if strings.HasPrefix(v, "es") {
		return "es"
	}
	return "en"
}

func tr(en, es string) string {
	if language() == "es" {
		return es
	}
	return en
}

func translatedStage(stage string) string {
	translations := map[string]string{
		"Downloading PHP archive":            "Descargando archivo PHP",
		"Using verified cached archive":      "Usando archivo verificado de caché",
		"Verifying SHA-256 checksum":         "Verificando checksum SHA-256",
		"Extracting archive":                 "Extrayendo archivo",
		"Configuring php.ini and extensions": "Configurando php.ini y extensiones",
		"Validating PHP runtime":             "Validando runtime de PHP",
		"Publishing installation":            "Publicando instalación",
	}
	if language() == "es" && translations[stage] != "" {
		return translations[stage]
	}
	return stage
}

// FriendlyError adds one actionable hint while preserving the original error text.
func FriendlyError(err error) string {
	message := err.Error()
	lower := strings.ToLower(message)
	hint := ""
	switch {
	case strings.Contains(lower, "not installed") || strings.Contains(lower, "no installed php"):
		hint = tr("run 'phpvm install <version>' or open 'phpvm ui'", "ejecuta 'phpvm install <versión>' o abre 'phpvm ui'")
	case strings.Contains(lower, "not trusted") || strings.Contains(lower, "trust is missing"):
		hint = tr("review the project files, then run 'phpvm trust project'", "revisa los archivos del proyecto y ejecuta 'phpvm trust project'")
	case strings.Contains(lower, "no active php version"):
		hint = tr("run 'phpvm use <version>'", "ejecuta 'phpvm use <versión>'")
	case strings.Contains(lower, "phpvm_safe_mode"):
		hint = tr("unset PHPVM_SAFE_MODE only if you intend to allow project execution", "quita PHPVM_SAFE_MODE solo si deseas permitir ejecución del proyecto")
	case strings.Contains(lower, "usage:"):
		hint = tr("run 'phpvm help' to see examples", "ejecuta 'phpvm help' para ver ejemplos")
	case strings.Contains(lower, "--version is required"):
		hint = tr("run 'phpvm init --version 8.4 --preset development'", "ejecuta 'phpvm init --version 8.4 --preset development'")
	}
	if hint == "" {
		return message
	}
	return message + "\n  " + tr("Hint", "Sugerencia") + ": " + hint
}

func (a *App) reader() *bufio.Reader {
	if a.input == nil {
		in := a.In
		if in == nil {
			in = os.Stdin
		}
		a.input = bufio.NewReader(in)
	}
	return a.input
}

func (a *App) requireInteractive() error {
	if !a.canPrompt() {
		return fmt.Errorf("%s", tr("interactive input requires a terminal; pass explicit arguments instead", "la entrada interactiva requiere una terminal; proporciona argumentos explícitos"))
	}
	return nil
}

func (a *App) canPrompt() bool {
	if a.ui == nil || !a.ui.interactive {
		return false
	}
	f, ok := a.In.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

func (a *App) promptText(label, defaultValue string) (string, error) {
	if defaultValue != "" {
		fmt.Fprintf(a.Out, "%s [%s]: ", label, defaultValue)
	} else {
		fmt.Fprintf(a.Out, "%s: ", label)
	}
	value, err := a.reader().ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		value = defaultValue
	}
	return value, nil
}

func (a *App) selectOne(title string, options []string) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("%s", tr("there are no available options", "no hay opciones disponibles"))
	}
	visible := options
	for {
		if a.ui != nil {
			a.ui.Section(a.ui.symbol("◆", "=="), title)
		} else {
			fmt.Fprintln(a.Out, title)
		}
		for i, option := range visible {
			number := fmt.Sprintf("%2d", i+1)
			if a.ui != nil {
				number = a.ui.paint(ansiCyan, number)
			}
			fmt.Fprintf(a.Out, "  %s  %s\n", number, option)
		}
		value, err := a.promptText(tr("Choose a number or type to filter", "Elige un número o escribe para filtrar"), "1")
		if err != nil {
			return "", err
		}
		selected, numberErr := strconv.Atoi(value)
		if numberErr == nil && selected >= 1 && selected <= len(visible) {
			return visible[selected-1], nil
		}
		needle := strings.ToLower(value)
		filtered := make([]string, 0)
		for _, option := range options {
			if strings.Contains(strings.ToLower(option), needle) {
				filtered = append(filtered, option)
			}
		}
		if len(filtered) == 1 {
			return filtered[0], nil
		}
		if len(filtered) == 0 {
			return "", fmt.Errorf("%s", tr("invalid selection", "selección inválida"))
		}
		visible = filtered
	}
}

func (a *App) confirm(question string, defaultYes bool) (bool, error) {
	hint := "y/N"
	if defaultYes {
		hint = "Y/n"
	}
	value, err := a.promptText(question+" ("+hint+")", "")
	if err != nil {
		return false, err
	}
	if value == "" {
		return defaultYes, nil
	}
	value = strings.ToLower(value)
	return value == "y" || value == "yes" || value == "s" || value == "si" || value == "sí", nil
}

func withoutYes(args []string) ([]string, bool) {
	out := make([]string, 0, len(args))
	yes := false
	for _, arg := range args {
		if arg == "--yes" || arg == "-y" {
			yes = true
		} else {
			out = append(out, arg)
		}
	}
	return out, yes
}

func (a *App) interactiveDoctor(s *store.Store) error {
	if err := a.requireInteractive(); err != nil {
		return err
	}
	err := a.doctor(s, nil)
	if err == nil {
		return nil
	}
	confirmed, askErr := a.confirm(tr("Apply safe repairs?", "¿Aplicar reparaciones seguras?"), true)
	if askErr != nil {
		return askErr
	}
	if !confirmed {
		return err
	}
	return a.doctorFix(s)
}

func (a *App) chooseInstalled(s *store.Store) (string, error) {
	builds, err := s.Installed()
	if err != nil {
		return "", err
	}
	options := make([]string, 0, len(builds))
	for _, build := range builds {
		options = append(options, build.ID())
	}
	return a.selectOne(tr("Installed PHP versions", "Versiones PHP instaladas"), options)
}

func (a *App) chooseRemote(ctx context.Context, s *store.Store, o buildOptions) (string, error) {
	p, err := provider(s.Root)
	if err != nil {
		return "", err
	}
	p.SetOffline(o.offline)
	stop := func(bool) {}
	if a.ui != nil {
		stop = a.ui.Spinner(tr("Loading official PHP catalog...", "Consultando catálogo oficial de PHP..."))
	}
	releases, err := p.Versions(ctx, o.variant, o.arch)
	stop(err == nil)
	if err != nil {
		return "", err
	}
	options := make([]string, 0, len(releases))
	for _, release := range releases {
		label := release.Version
		if windowsphp.IsEOL(release.Version, now()) {
			label += " (EOL)"
		}
		options = append(options, label)
	}
	selected, err := a.selectOne(tr("Available PHP branches", "Ramas PHP disponibles"), options)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(selected, " (EOL)"), nil
}

func now() time.Time { return time.Now() }

func (a *App) dashboard(s *store.Store, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: phpvm dashboard")
	}
	a.ui.Banner(a.Version, tr("Your PHP workspace at a glance", "Tu entorno PHP de un vistazo"))
	if err := a.status(s, nil); err != nil {
		a.ui.Info(tr("No PHP is selected yet. Next: phpvm install", "Aún no hay un PHP seleccionado. Siguiente: phpvm install"))
	}
	return nil
}

func (a *App) interactiveUI(ctx context.Context, s *store.Store, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: phpvm ui")
	}
	if err := a.requireInteractive(); err != nil {
		return err
	}
	a.ui.Banner(a.Version, tr("Interactive PHP workspace", "Centro de trabajo PHP interactivo"))
	dashboardLabel := a.ui.symbol("📊  ", "") + tr("Dashboard", "Panel de estado")
	activateLabel := a.ui.symbol("⚡  ", "") + tr("Activate installed PHP", "Activar PHP instalado")
	installLabel := a.ui.symbol("📦  ", "") + tr("Install a PHP version", "Instalar una versión PHP")
	initLabel := a.ui.symbol("🧭  ", "") + tr("Initialize this project", "Inicializar este proyecto")
	doctorLabel := a.ui.symbol("🩺  ", "") + tr("Run diagnostics", "Ejecutar diagnóstico")
	logsLabel := a.ui.symbol("📄  ", "") + tr("Open error log", "Abrir log de errores")
	exitLabel := a.ui.symbol("↩  ", "") + tr("Exit", "Salir")
	for {
		choice, err := a.selectOne(tr("phpvm control center", "Centro de control phpvm"), []string{
			dashboardLabel, activateLabel, installLabel, initLabel, doctorLabel, logsLabel, exitLabel,
		})
		if err != nil {
			return err
		}
		switch choice {
		case dashboardLabel:
			_ = a.dashboard(s, nil)
		case activateLabel:
			id, e := a.chooseInstalled(s)
			if e == nil {
				e = s.Use(id)
			}
			if e != nil {
				a.ui.Warn("%v", e)
			} else {
				a.ui.Success(tr("Using PHP %s", "Usando PHP %s"), id)
			}
		case installLabel:
			version, e := a.chooseRemote(ctx, s, defaultOptions())
			if e == nil {
				_, e = a.install(ctx, s, version, defaultOptions())
			}
			if e != nil {
				a.ui.Warn("%v", e)
			}
		case initLabel:
			if e := a.initProject(s, nil); e != nil {
				a.ui.Warn("%v", e)
			}
		case doctorLabel:
			if e := a.doctor(s, nil); e != nil {
				fix, askErr := a.confirm(tr("Apply safe repairs?", "¿Aplicar reparaciones seguras?"), true)
				if askErr != nil {
					return askErr
				}
				if fix {
					_ = a.doctorFix(s)
				}
			}
		case logsLabel:
			if e := a.openTarget(s, []string{"logs"}); e != nil {
				a.ui.Warn("%v", e)
			}
		default:
			return nil
		}
	}
}

func (a *App) initProject(s *store.Store, args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	version := fs.String("version", "", "")
	preset := fs.String("preset", "", "")
	force := fs.Bool("force", false, "")
	if err := fs.Parse(args); err != nil || len(fs.Args()) != 0 {
		return fmt.Errorf("usage: phpvm init [--version <version>] [--preset <name>] [--force]")
	}
	interactive := a.canPrompt()
	if *version == "" {
		if !interactive {
			return fmt.Errorf("--version is required outside an interactive terminal")
		}
		fallback := "8.4"
		if id, err := s.Current(); err == nil {
			if m, metaErr := s.Metadata(id); metaErr == nil {
				fallback = m.Version
			}
		}
		value, err := a.promptText(tr("PHP version or constraint", "Versión o restricción PHP"), fallback)
		if err != nil {
			return err
		}
		*version = value
	}
	if *preset == "" && interactive {
		value, err := a.selectOne(tr("INI preset", "Preset de INI"), []string{"development", "production", "testing", "codeigniter", "laravel", "wordpress"})
		if err != nil {
			return err
		}
		*preset = value
	}
	settings := map[string]string{}
	if *preset != "" {
		var ok bool
		settings, ok = presetSettings(*preset)
		if !ok {
			return fmt.Errorf("unknown preset %s", *preset)
		}
	}
	path := filepath.Join(currentDir(), "phpvm.toml")
	if _, err := os.Stat(path); err == nil && !*force {
		return fmt.Errorf("phpvm.toml already exists; use --force to replace it")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "version = %q\nvariant = %q\narch = %q\n", *version, "nts", defaultOptions().arch)
	if len(settings) > 0 {
		b.WriteString("\n[ini]\n")
		keys := make([]string, 0, len(settings))
		for key := range settings {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintf(&b, "%s = %q\n", key, settings[key])
		}
	}
	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		return err
	}
	a.ui.Success(tr("Created %s", "Creado %s"), path)
	a.ui.Info(tr("Next: review the file, run 'phpvm trust project', then 'phpvm sync'", "Siguiente: revisa el archivo, ejecuta 'phpvm trust project' y después 'phpvm sync'"))
	return nil
}

func presetSettings(name string) (map[string]string, bool) {
	presets := map[string]map[string]string{
		"development": {"display_errors": "On", "display_startup_errors": "On", "error_reporting": "E_ALL", "memory_limit": "512M"},
		"production":  {"display_errors": "Off", "display_startup_errors": "Off", "error_reporting": "E_ALL & ~E_DEPRECATED & ~E_STRICT", "memory_limit": "256M", "opcache.enable": "1"},
		"testing":     {"display_errors": "On", "error_reporting": "E_ALL", "memory_limit": "1G", "max_execution_time": "0"},
		"codeigniter": {"display_errors": "On", "memory_limit": "512M"},
		"laravel":     {"display_errors": "On", "memory_limit": "512M"},
		"wordpress":   {"display_errors": "Off", "memory_limit": "256M"},
	}
	settings, ok := presets[strings.ToLower(name)]
	return settings, ok
}

func (a *App) openTarget(s *store.Store, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: phpvm open <logs|ini|root>")
	}
	var path string
	var err error
	switch args[0] {
	case "logs", "log":
		path, _, err = ensureLogConfig(s)
	case "ini":
		_, dir, targetErr := activeDir(s)
		if targetErr != nil {
			return targetErr
		}
		path, err = ensureINI(dir)
	case "root":
		path = s.Root
		if err = os.MkdirAll(path, 0755); err != nil {
			return err
		}
	default:
		return fmt.Errorf("usage: phpvm open <logs|ini|root>")
	}
	if err != nil {
		return err
	}
	a.ui.Info(tr("Opening %s", "Abriendo %s"), path)
	return openFile(path)
}
