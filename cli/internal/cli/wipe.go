package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mgeovany/sentra/cli/internal/storage"
	"github.com/zalando/go-keyring"
)

func runWipe(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: sentra wipe")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	sentraDir := filepath.Join(homeDir, ".sentra")

	if err := confirmWipe(sentraDir); err != nil {
		return err
	}

	// Best-effort: delete storage secret (keychain) using config, if present.
	_ = storage.DeleteConfig()

	// Best-effort: delete keychain items.
	_ = keyring.Delete("sentra", "session")
	_ = keyring.Delete("sentra", "session-key")
	_ = keyring.Delete("sentra", "device-ed25519")

	// Remove all local state.
	if err := os.RemoveAll(sentraDir); err != nil {
		return err
	}

	fmt.Println("✔ wiped local Sentra state")
	fmt.Printf("✔ deleted %s\n", sentraDir)
	return nil
}

func confirmWipe(sentraDir string) error {
	sentraDir = strings.TrimSpace(sentraDir)
	if sentraDir == "" {
		return errors.New("invalid state dir")
	}

	// Refuse in non-interactive environments.
	fi, err := os.Stdin.Stat()
	if err == nil {
		if (fi.Mode() & os.ModeCharDevice) == 0 {
			return errors.New("refusing to wipe without a TTY")
		}
	}

	fmt.Println("This will delete ALL local Sentra data:")
	fmt.Printf("- %s (commits, index, config, cache)\n", sentraDir)
	fmt.Println("- keychain items: sentra/session, sentra/session-key, sentra/device-ed25519")
	fmt.Println()
	fmt.Print("Type WIPE to continue: ")

	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	if strings.TrimSpace(line) != "WIPE" {
		return errors.New("wipe cancelled")
	}
	return nil
}
