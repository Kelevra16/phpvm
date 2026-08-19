package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/megaj/phpvm/internal/store"
)

func activeDir(s *store.Store) (string, string, error) {
	id, err := s.Current()
	if err != nil {
		return "", "", err
	}
	return id, filepath.Dir(s.Executable(id)), nil
}
func ensureINI(dir string) (string, error) {
	path := filepath.Join(dir, "php.ini")
	if _, err := os.Stat(path); err == nil {
		return path, setINI(path, "extension_dir", `"`+filepath.Join(dir, "ext")+`"`)
	}
	for _, template := range []string{"php.ini-development", "php.ini-production"} {
		b, err := os.ReadFile(filepath.Join(dir, template))
		if err == nil {
			if err := os.WriteFile(path, b, 0644); err != nil {
				return path, err
			}
			return path, setINI(path, "extension_dir", `"`+filepath.Join(dir, "ext")+`"`)
		}
	}
	if err := os.WriteFile(path, nil, 0644); err != nil {
		return path, err
	}
	return path, setINI(path, "extension_dir", `"`+filepath.Join(dir, "ext")+`"`)
}
func (a *App) ini(s *store.Store, args []string) error {
	_, dir, err := activeDir(s)
	if err != nil {
		return err
	}
	path, err := ensureINI(dir)
	if err != nil {
		return err
	}
	if len(args) == 3 && args[0] == "set" {
		if err := setINI(path, args[1], args[2]); err != nil {
			return err
		}
		fmt.Fprintln(a.Out, args[1], "=", args[2])
		return nil
	}
	if len(args) == 2 && args[0] == "get" {
		values, err := readINI(path)
		if err != nil {
			return err
		}
		v, ok := values[args[1]]
		if !ok {
			return fmt.Errorf("%s is not set", args[1])
		}
		fmt.Fprintln(a.Out, v)
		return nil
	}
	return fmt.Errorf("usage: phpvm ini get <key> | phpvm ini set <key> <value>")
}
func readINI(path string) (map[string]string, error) {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		p := strings.SplitN(line, "=", 2)
		if len(p) == 2 {
			out[strings.TrimSpace(p[0])] = strings.TrimSpace(p[1])
		}
	}
	return out, sc.Err()
}

type profiles map[string]map[string]string

func profilePath(s *store.Store) string { return filepath.Join(s.Root, "profiles.json") }
func loadProfiles(s *store.Store) (profiles, error) {
	p := profiles{}
	b, err := os.ReadFile(profilePath(s))
	if os.IsNotExist(err) {
		return p, nil
	}
	if err != nil {
		return nil, err
	}
	return p, json.Unmarshal(b, &p)
}
func (a *App) profile(s *store.Store, args []string) error {
	p, err := loadProfiles(s)
	if err != nil {
		return err
	}
	if len(args) == 0 || args[0] == "ls" {
		names := make([]string, 0, len(p))
		for n := range p {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			fmt.Fprintln(a.Out, n)
		}
		return nil
	}
	if len(args) == 2 && args[0] == "create" {
		if _, ok := p[args[1]]; ok {
			return fmt.Errorf("profile %s already exists", args[1])
		}
		p[args[1]] = map[string]string{}
		return writeJSON(profilePath(s), p)
	}
	if len(args) == 4 && args[0] == "set" {
		if _, ok := p[args[1]]; !ok {
			p[args[1]] = map[string]string{}
		}
		p[args[1]][args[2]] = args[3]
		return writeJSON(profilePath(s), p)
	}
	if len(args) == 2 && args[0] == "use" {
		values, ok := p[args[1]]
		if !ok {
			return fmt.Errorf("profile %s does not exist", args[1])
		}
		_, dir, err := activeDir(s)
		if err != nil {
			return err
		}
		path, err := ensureINI(dir)
		if err != nil {
			return err
		}
		for k, v := range values {
			if err := setINI(path, k, v); err != nil {
				return err
			}
		}
		fmt.Fprintln(a.Out, "Applied profile", args[1])
		return nil
	}
	return fmt.Errorf("usage: phpvm profile ls|create <name>|set <name> <key> <value>|use <name>")
}

func (a *App) extensions(s *store.Store, args []string) error {
	_, dir, err := activeDir(s)
	if err != nil {
		return err
	}
	path, err := ensureINI(dir)
	if err != nil {
		return err
	}
	if len(args) == 0 || args[0] == "ls" {
		entries, err := os.ReadDir(filepath.Join(dir, "ext"))
		if err != nil {
			return err
		}
		enabled, _ := enabledExtensions(path)
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".dll") {
				continue
			}
			name := strings.TrimSuffix(strings.TrimPrefix(e.Name(), "php_"), ".dll")
			mark := "  "
			if enabled[name] {
				mark = "* "
			}
			fmt.Fprintln(a.Out, mark+name)
		}
		return nil
	}
	if len(args) == 2 && (args[0] == "enable" || args[0] == "disable") {
		name := strings.TrimSuffix(strings.TrimPrefix(args[1], "php_"), ".dll")
		dll := "php_" + name + ".dll"
		if _, err := os.Stat(filepath.Join(dir, "ext", dll)); err != nil {
			return fmt.Errorf("extension %s is not included in this PHP build", name)
		}
		if err := toggleExtension(path, name, args[0] == "enable"); err != nil {
			return err
		}
		fmt.Fprintln(a.Out, args[0]+"d", name)
		return nil
	}
	return fmt.Errorf("usage: phpvm ext ls|enable <name>|disable <name>")
}
func enabledExtensions(path string) (map[string]bool, error) {
	out := map[string]bool{}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "extension=") || strings.HasPrefix(line, "extension =") {
			v := strings.TrimSpace(strings.SplitN(line, "=", 2)[1])
			v = strings.Trim(v, "\"")
			out[strings.TrimSuffix(strings.TrimPrefix(v, "php_"), ".dll")] = true
		}
	}
	return out, sc.Err()
}
func toggleExtension(path, name string, enable bool) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	dll := "php_" + name + ".dll"
	found := false
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		plain := strings.TrimSpace(strings.TrimPrefix(trim, ";"))
		if strings.HasPrefix(plain, "extension=") || strings.HasPrefix(plain, "extension =") {
			v := strings.Trim(strings.TrimSpace(strings.SplitN(plain, "=", 2)[1]), "\"")
			if v == dll || v == name {
				found = true
				if enable {
					lines[i] = "extension=" + dll
				} else {
					lines[i] = ";extension=" + dll
				}
			}
		}
	}
	if enable && !found {
		lines = append(lines, "extension="+dll)
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\r\n")), 0644)
}
