package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

func promptSecret(r *bufio.Reader, label string) (string, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		label = "Secret"
	}

	// Non-TTY: cannot safely hide input.
	if !isTTY(os.Stdout) || !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", errors.New("cannot prompt for secret without a TTY (set SENTRA_VAULT_PASSPHRASE env var)")
	}

	fmt.Print(label + ": ")
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(string(b))
	if v == "" {
		return "", errors.New("value required")
	}
	// Drain any buffered input.
	_, _ = r.ReadString('\n')
	return v, nil
}
