package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

func promptBox(r *bufio.Reader, title string, hint string, defaultValue string) (string, error) {
	title = strings.TrimSpace(title)
	hint = strings.TrimSpace(hint)
	defaultValue = strings.TrimSpace(defaultValue)

	// Non-TTY: keep it simple.
	if !isTTY(os.Stdout) {
		if defaultValue != "" {
			fmt.Printf("%s [%s]: ", title, defaultValue)
		} else {
			fmt.Printf("%s: ", title)
		}
		line, err := r.ReadString('\n')
		if err != nil {
			return "", err
		}
		v := strings.TrimSpace(line)
		if v == "" {
			v = defaultValue
		}
		return v, nil
	}

	inner := 66
	border := "+" + strings.Repeat("-", inner) + "+"
	inputLinePrefix := "| "
	inputLineSuffix := " |"
	contentWidth := inner - 2

	if title != "" {
		fmt.Println(c(ansiBoldCyan, title))
	}
	if hint != "" {
		fmt.Println(c(ansiDim, hint))
	}
	if defaultValue != "" {
		fmt.Println(c(ansiDim, "Default: "+defaultValue+" (press Enter)"))
	}

	fmt.Println(c(ansiDim, border))
	fmt.Print(c(ansiDim, inputLinePrefix))

	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(line)
	if v == "" {
		v = defaultValue
	}

	// Re-render the input line with a closing border.
	// Move cursor up to the input line, clear it, and print a padded/truncated version.
	shown := v
	if len(shown) > contentWidth {
		if contentWidth >= 3 {
			shown = shown[:contentWidth-3] + "..."
		} else {
			shown = shown[:contentWidth]
		}
	}
	pad := ""
	if n := contentWidth - len(shown); n > 0 {
		pad = strings.Repeat(" ", n)
	}

	// up one line
	_, _ = fmt.Fprint(os.Stdout, "\x1b[1A")
	// clear line
	_, _ = fmt.Fprint(os.Stdout, "\r\x1b[2K")
	_, _ = fmt.Fprint(os.Stdout, c(ansiDim, inputLinePrefix)+shown+pad+c(ansiDim, inputLineSuffix))
	// down one line
	_, _ = fmt.Fprint(os.Stdout, "\x1b[1B")

	fmt.Println(c(ansiDim, border))

	v = strings.TrimSpace(v)
	if v == "" {
		return "", errors.New("value required")
	}
	return v, nil
}
