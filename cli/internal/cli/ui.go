package cli

import (
	"fmt"
	"os"
	"strings"
)

type ansiCode string

const (
	ansiReset    ansiCode = "\x1b[0m"
	ansiDim      ansiCode = "\x1b[2m"
	ansiBold     ansiCode = "\x1b[1m"
	ansiRed      ansiCode = "\x1b[31m"
	ansiGreen    ansiCode = "\x1b[32m"
	ansiYellow   ansiCode = "\x1b[33m"
	ansiCyan     ansiCode = "\x1b[36m"
	ansiBoldCyan ansiCode = "\x1b[1;36m"
)

func colorEnabled() bool {
	if strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		return false
	}
	v := strings.TrimSpace(os.Getenv("SENTRA_NO_COLOR"))
	if v == "1" || strings.EqualFold(v, "true") {
		return false
	}
	term := strings.TrimSpace(os.Getenv("TERM"))
	if term == "" || term == "dumb" {
		return false
	}
	return isTTY(os.Stdout)
}

func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func c(code ansiCode, s string) string {
	if !colorEnabled() {
		return s
	}
	return string(code) + s + string(ansiReset)
}

func formatRecommendedLabel(label string, muted bool) string {
	const tag = "(recommended)"
	idx := strings.Index(label, tag)
	if idx < 0 {
		if muted {
			return c(ansiDim, label)
		}
		return label
	}
	pre := label[:idx]
	post := label[idx+len(tag):]
	if muted {
		return c(ansiDim, pre) + c(ansiGreen, tag) + c(ansiDim, post)
	}
	return pre + c(ansiGreen, tag) + post
}

func infof(format string, args ...any) {
	_, _ = fmt.Fprintln(os.Stdout, c(ansiDim, fmt.Sprintf(format, args...)))
}

func successf(format string, args ...any) {
	_, _ = fmt.Fprintln(os.Stdout, c(ansiGreen, fmt.Sprintf(format, args...)))
}

func warnf(format string, args ...any) {
	_, _ = fmt.Fprintln(os.Stdout, c(ansiYellow, fmt.Sprintf(format, args...)))
}

func verbosef(format string, args ...any) {
	if !isVerbose() {
		return
	}
	_, _ = fmt.Fprintln(os.Stdout, c(ansiDim, fmt.Sprintf(format, args...)))
}
