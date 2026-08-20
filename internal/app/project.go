package app

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type projectConfig struct {
	Version, Variant, Arch, Source string
	INI                            map[string]string
	LogScope, LogPath              string
}

var phpConstraint = regexp.MustCompile(`(?:\^|~|>=?\s*)?(\d+\.\d+)(?:\.\d+)?`)

func findProjectConfig() (projectConfig, error) {
	dir, err := os.Getwd()
	if err != nil {
		return projectConfig{}, err
	}
	for {
		if c, ok, err := readProjectDir(dir); err != nil {
			return c, err
		} else if ok {
			return c, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return projectConfig{}, nil
		}
		dir = parent
	}
}
func readProjectDir(dir string) (projectConfig, bool, error) {
	if b, err := os.ReadFile(filepath.Join(dir, ".php-version")); err == nil {
		return projectConfig{Version: strings.TrimSpace(string(b)), Source: filepath.Join(dir, ".php-version"), INI: map[string]string{}}, true, nil
	} else if !os.IsNotExist(err) {
		return projectConfig{}, false, err
	}
	if b, err := os.ReadFile(filepath.Join(dir, "phpvm.toml")); err == nil {
		c := parseTOML(string(b))
		c.Source = filepath.Join(dir, "phpvm.toml")
		return c, c.Version != "", nil
	} else if !os.IsNotExist(err) {
		return projectConfig{}, false, err
	}
	for _, name := range []string{"composer.lock", "composer.json"} {
		if b, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
			var data map[string]any
			if json.Unmarshal(b, &data) == nil {
				if p, ok := data["platform"].(map[string]any); ok {
					if v, ok := p["php"].(string); ok {
						return projectConfig{Version: constraintVersion(v), Source: name, INI: map[string]string{}}, true, nil
					}
				}
				if req, ok := data["require"].(map[string]any); ok {
					if v, ok := req["php"].(string); ok {
						return projectConfig{Version: constraintVersion(v), Source: name, INI: map[string]string{}}, true, nil
					}
				}
			}
		}
	}
	return projectConfig{}, false, nil
}
func constraintVersion(v string) string {
	m := phpConstraint.FindStringSubmatch(v)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}
func parseTOML(text string) projectConfig {
	c := projectConfig{INI: map[string]string{}}
	section := ""
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		line := strings.TrimSpace(strings.SplitN(sc.Text(), "#", 2)[0])
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.Trim(line, "[] ")
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		v := strings.Trim(strings.TrimSpace(parts[1]), "\"")
		if section == "ini" {
			c.INI[k] = v
			continue
		}
		if section == "logs" {
			if k == "scope" {
				c.LogScope = v
			}
			if k == "path" {
				c.LogPath = v
			}
			continue
		}
		switch k {
		case "version":
			c.Version = v
		case "variant":
			c.Variant = v
		case "arch":
			c.Arch = v
		}
	}
	return c
}
func setINI(path, key, value string) error {
	lines := []string{}
	if b, err := os.ReadFile(path); err == nil {
		lines = strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	}
	found := false
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, key+"=") || strings.HasPrefix(trim, key+" =") {
			lines[i] = key + " = " + value
			found = true
		}
	}
	if !found {
		lines = append(lines, key+" = "+value)
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\r\n")), 0644)
}
