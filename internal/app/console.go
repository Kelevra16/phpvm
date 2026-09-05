package app

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	ansiReset  = "\x1b[0m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiRed    = "\x1b[31m"
	ansiCyan   = "\x1b[36m"
	ansiBlue   = "\x1b[34m"
	ansiPurple = "\x1b[35m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
)

type console struct {
	out, err                             io.Writer
	color, unicode, verbose, interactive bool
	progressLast                         int
}

func (c *console) Clear() {
	if c.interactive && c.unicode {
		fmt.Fprint(c.out, "\x1b[2J\x1b[H")
	}
}

func (c *console) KeyValue(label, value string) {
	fmt.Fprintf(c.out, "  %s  %s\n", c.paint(ansiDim, fmt.Sprintf("%-15s", label)), value)
}

func (c *console) MenuItem(number int, icon, label, description string) {
	key := c.paint(ansiPurple+ansiBold, fmt.Sprintf("[%d]", number))
	fmt.Fprintf(c.out, "  %s  %s %s\n", key, icon, c.paint(ansiBold, label))
	fmt.Fprintf(c.out, "       %s\n\n", c.paint(ansiDim, description))
}

func (c *console) Banner(version, subtitle string) {
	if !c.unicode {
		fmt.Fprintf(c.out, "phpvm %s - %s\n\n", version, subtitle)
		return
	}
	logo := []string{
		"╭─ 🐘  PHPVM  " + version + "  ─────────────────────",
		"│  " + subtitle,
		"╰──────────────────────────────────────────────",
	}
	for i, line := range logo {
		color := ansiPurple
		if i == 1 {
			color = ansiCyan
		}
		fmt.Fprintln(c.out, c.paint(color, line))
	}
}

func (c *console) Section(icon, title string) {
	if !c.unicode {
		icon = "=="
	}
	fmt.Fprintln(c.out)
	fmt.Fprintln(c.out, c.paint(ansiBold+ansiBlue, icon+"  "+title))
}

func (c *console) Command(command, description string) {
	prefix := c.symbol("›", ">")
	const commandWidth = 36
	if len(command) > commandWidth {
		fmt.Fprintf(c.out, "  %s %s\n", c.paint(ansiCyan, prefix), c.paint(ansiGreen, command))
		for _, line := range wrapWords(description, 66) {
			fmt.Fprintf(c.out, "      %s\n", c.paint(ansiDim, line))
		}
		return
	}
	lines := wrapWords(description, 34)
	paddedCommand := fmt.Sprintf("%-*s", commandWidth, command)
	fmt.Fprintf(c.out, "  %s %s  %s\n", c.paint(ansiCyan, prefix), c.paint(ansiGreen, paddedCommand), c.paint(ansiDim, lines[0]))
	for _, line := range lines[1:] {
		fmt.Fprintf(c.out, "      %-*s  %s\n", commandWidth, "", c.paint(ansiDim, line))
	}
}

func wrapWords(text string, width int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	lines := []string{words[0]}
	for _, word := range words[1:] {
		last := len(lines) - 1
		if len(lines[last])+1+len(word) <= width {
			lines[last] += " " + word
		} else {
			lines = append(lines, word)
		}
	}
	return lines
}

// Spinner animates only on a real decorated terminal. The returned function
// stops it and paints a final success or failure marker.
func (c *console) Spinner(label string) func(bool) {
	if !c.interactive || !c.unicode {
		fmt.Fprintln(c.out, "-> "+label)
		return func(bool) {}
	}
	stop := make(chan bool, 1)
	done := make(chan struct{})
	var once sync.Once
	go func() {
		defer close(done)
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		i := 0
		for {
			fmt.Fprintf(c.out, "\r  %s %s", c.paint(ansiPurple, frames[i%len(frames)]), label)
			select {
			case ok := <-stop:
				mark, color := "×", ansiRed
				if ok {
					mark, color = "✓", ansiGreen
				}
				fmt.Fprintf(c.out, "\r  %s %s\x1b[K\n", c.paint(color, mark), label)
				return
			case <-ticker.C:
				i++
			}
		}
	}()
	return func(ok bool) {
		once.Do(func() { stop <- ok; <-done })
	}
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
func (c *console) Step(format string, v ...any) {
	fmt.Fprintln(c.out, "  "+c.paint(ansiCyan, c.symbol("◆", ">"))+" "+fmt.Sprintf(format, v...))
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
