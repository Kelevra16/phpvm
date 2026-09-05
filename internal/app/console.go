package app

import (
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	ansiReset  = "\x1b[0m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiRed    = "\x1b[31m"
	ansiCyan   = "\x1b[36m"
	ansiDim    = "\x1b[2m"
)

type console struct {
	out, err                             io.Writer
	color, unicode, verbose, interactive bool
	progressLast                         int
}

func newConsole(out, err io.Writer, plain, verbose bool) *console {
	interactive := isTerminal(out)
	color := interactive && !plain && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
	return &console{out: out, err: err, color: color, unicode: interactive && !plain, verbose: verbose, interactive: interactive, progressLast: -1}
}
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, e := f.Stat()
	return e == nil && (st.Mode()&os.ModeCharDevice) != 0
}
func (c *console) paint(code, s string) string {
	if !c.color {
		return s
	}
	return code + s + ansiReset
}
func (c *console) symbol(unicode, ascii string) string {
	if c.unicode {
		return unicode
	}
	return ascii
}
func (c *console) Success(format string, v ...any) {
	fmt.Fprintln(c.out, c.paint(ansiGreen, c.symbol("✓", "OK"))+" "+fmt.Sprintf(format, v...))
}
func (c *console) Info(format string, v ...any) {
	fmt.Fprintln(c.out, c.paint(ansiCyan, c.symbol("→", "->"))+" "+fmt.Sprintf(format, v...))
}
func (c *console) Warn(format string, v ...any) {
	fmt.Fprintln(c.err, c.paint(ansiYellow, c.symbol("!", "WARN"))+" "+fmt.Sprintf(format, v...))
}
func (c *console) Debug(format string, v ...any) {
	if c.verbose {
		fmt.Fprintln(c.err, c.paint(ansiDim, "debug:")+" "+fmt.Sprintf(format, v...))
	}
}
func (c *console) Progress(done, total int64) {
	if total <= 0 {
		return
	}
	p := int(done * 100 / total)
	if !c.interactive {
		if p/10 == c.progressLast/10 {
			return
		}
		c.progressLast = p
		fmt.Fprintf(c.out, "Downloading %d%%\n", p)
		return
	}
	width := 20
	filled := p * width / 100
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	if !c.unicode {
		bar = strings.Repeat("#", filled) + strings.Repeat("-", width-filled)
	}
	fmt.Fprintf(c.out, "\r  [%s] %3d%%  %.1f MB / %.1f MB", bar, p, float64(done)/1048576, float64(total)/1048576)
	if done >= total {
		fmt.Fprintln(c.out)
	}
}
func (c *console) Table(headers []string, rows [][]string) {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, v := range row {
			if i < len(widths) && len(v) > widths[i] {
				widths[i] = len(v)
			}
		}
	}
	for i, h := range headers {
		fmt.Fprintf(c.out, "%-*s", widths[i]+2, h)
	}
	fmt.Fprintln(c.out)
	for _, row := range rows {
		for i, v := range row {
			fmt.Fprintf(c.out, "%-*s", widths[i]+2, v)
		}
		fmt.Fprintln(c.out)
	}
}

func stripGlobalFlags(args []string) ([]string, bool, bool) {
	plain, verbose, passthrough := false, false, false
	out := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--" {
			passthrough = true
			out = append(out, arg)
			continue
		}
		if !passthrough {
			switch arg {
			case "--no-color", "--plain":
				plain = true
				continue
			case "--verbose":
				verbose = true
				continue
			}
		}
		out = append(out, arg)
	}
	return out, plain, verbose
}
