package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

func promptSelect(r *bufio.Reader, options []string) (int, error) {
	if len(options) == 0 {
		return 0, errors.New("no options")
	}

	// Interactive (arrow keys) only when we have a real terminal.
	if isTTY(os.Stdin) && isTTY(os.Stdout) {
		if n, err := promptSelectArrows(options); err == nil {
			return n, nil
		}
		// Fall back to numeric prompt on any error.
	}

	return promptSelectNumeric(r, options)
}

func promptSelectNumeric(r *bufio.Reader, options []string) (int, error) {
	def := 1
	fmt.Println() // Blank line for separation
	for i, o := range options {
		n := i + 1
		label := formatRecommendedLabel(o, false)
		marker := "  "
		if n == def {
			marker = c(ansiGreen, ">") + " "
		}
		fmt.Printf("%s%s %s\n", marker, c(ansiCyan, fmt.Sprintf("%d)", n)), label)
	}
	fmt.Println(c(ansiDim, "(Type a number and press Enter, or just Enter for default)"))

	for {
		fmt.Print(c(ansiDim, fmt.Sprintf("Choice [%d]: ", def)))
		line, err := r.ReadString('\n')
		if err != nil {
			return 0, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return def, nil
		}
		n := 0
		for _, ch := range line {
			if ch < '0' || ch > '9' {
				n = 0
				break
			}
			n = n*10 + int(ch-'0')
		}
		if n >= 1 && n <= len(options) {
			return n, nil
		}
		fmt.Println(c(ansiYellow, "Invalid choice. Please enter a number between 1 and "+fmt.Sprintf("%d", len(options))))
	}
}

func promptSelectArrows(options []string) (int, error) {
	fd := int(os.Stdin.Fd())
	old, err := term.MakeRaw(fd)
	if err != nil {
		return 0, err
	}
	defer func() { _ = term.Restore(fd, old) }()

	writeLine := func(s string) {
		// Use CRLF to avoid terminals where LF doesn't reset column.
		_, _ = fmt.Fprint(os.Stdout, "\r"+s+"\r\n")
	}

	// Hide cursor.
	_, _ = fmt.Fprint(os.Stdout, "\x1b[?25l")
	defer func() { _, _ = fmt.Fprint(os.Stdout, "\x1b[?25h") }()

	sel := 0
	// Print a blank line first for better separation
	_, _ = fmt.Fprint(os.Stdout, "\r\n")
	// Save cursor at the start of the menu so we can redraw reliably.
	// Prefer DEC save/restore (ESC7/ESC8); widely supported.
	_, _ = fmt.Fprint(os.Stdout, "\r\x1b7")

	redraw := func() {
		// Restore cursor to the saved position, clear down, then print.
		_, _ = fmt.Fprint(os.Stdout, "\x1b8\r\x1b[J")

		width, _, werr := term.GetSize(fd)
		if werr != nil || width <= 0 {
			width = 80
		}
		// Avoid line wrapping; keep output within terminal width.
		maxLine := width - 1

		for i, o := range options {
			n := i + 1
			prefixPlain := "  "
			numPlain := fmt.Sprintf("%d) ", n)
			labelPlain := strings.TrimSpace(o)

			// Truncate label to fit the terminal.
			avail := maxLine - len(prefixPlain) - len(numPlain)
			if avail < 0 {
				avail = 0
			}
			if len(labelPlain) > avail {
				if avail >= 3 {
					labelPlain = labelPlain[:avail-3] + "..."
				} else {
					labelPlain = labelPlain[:avail]
				}
			}

			if i == sel {
				prefix := c(ansiGreen, ">") + " "
				num := c(ansiBoldCyan, numPlain)
				label := formatRecommendedLabel(labelPlain, false)
				writeLine(prefix + num + label)
				continue
			}
			prefix := c(ansiDim, prefixPlain)
			num := c(ansiCyan, numPlain)
			label := formatRecommendedLabel(labelPlain, true)
			writeLine(prefix + num + label)
		}
		writeLine(c(ansiDim, "Use Up/Down to move, Enter to select, or type a number"))
	}

	redraw()

	buf := make([]byte, 1)
	var num string
	for {
		_, err := os.Stdin.Read(buf)
		if err != nil {
			return 0, err
		}
		b := buf[0]

		// Enter
		if b == '\r' || b == '\n' {
			if strings.TrimSpace(num) != "" {
				// numeric quick select
				val := 0
				for _, ch := range num {
					val = val*10 + int(ch-'0')
				}
				if val >= 1 && val <= len(options) {
					sel = val - 1
				}
			}
			return sel + 1, nil
		}

		// Ctrl+C
		if b == 3 {
			return 0, errors.New("cancelled")
		}

		// Digits: allow typing a number then Enter.
		if b >= '0' && b <= '9' {
			num += string(b)
			if len(num) > 4 {
				num = num[len(num)-4:]
			}
			continue
		}
		if b == 127 || b == 8 {
			if len(num) > 0 {
				num = num[:len(num)-1]
			}
			continue
		}
		num = ""

		// Arrow keys: ESC [ A/B
		if b == 0x1b {
			seq := make([]byte, 2)
			n1, _ := os.Stdin.Read(seq[:1])
			if n1 == 0 || seq[0] != '[' {
				continue
			}
			n2, _ := os.Stdin.Read(seq[1:2])
			if n2 == 0 {
				continue
			}
			switch seq[1] {
			case 'A': // Up
				if sel > 0 {
					sel--
					redraw()
				}
			case 'B': // Down
				if sel < len(options)-1 {
					sel++
					redraw()
				}
			}
			continue
		}

		// vim keys
		switch b {
		case 'k':
			if sel > 0 {
				sel--
				redraw()
			}
		case 'j':
			if sel < len(options)-1 {
				sel++
				redraw()
			}
		}
	}
}
