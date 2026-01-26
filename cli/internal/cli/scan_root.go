package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mgeovany/sentra/cli/internal/index"
)

func resolveScanRoot() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	defaultRoot := filepath.Join(homeDir, "dev")

	indexPath, err := index.DefaultPath()
	if err != nil {
		return defaultRoot, nil
	}
	idx, ok, err := index.Load(indexPath)
	if err != nil {
		return "", err
	}
	if ok {
		if v := strings.TrimSpace(idx.ScanRoot); v != "" {
			if isDir(v) {
				return v, nil
			}
		}
	}

	chosen, err := promptScanRoot(defaultRoot)
	if err != nil {
		return "", err
	}
	idx.ScanRoot = chosen
	if err := index.Save(indexPath, idx); err != nil {
		return "", err
	}
	return chosen, nil
}

func promptScanRoot(defaultRoot string) (string, error) {
	defaultRoot = strings.TrimSpace(defaultRoot)
	if defaultRoot == "" {
		return "", errors.New("default scan root is required")
	}

	r := bufio.NewReader(os.Stdin)
	for attempt := 0; attempt < 3; attempt++ {
		v, err := promptBox(r, "Where are your repos?", "Enter the folder that contains your git repos (e.g. ~/dev)", defaultRoot)
		if err != nil {
			// If stdin isn't readable (non-interactive), fall back to default.
			if isDir(defaultRoot) {
				return defaultRoot, nil
			}
			return "", err
		}
		v = expandUserHome(v)
		if !filepath.IsAbs(v) {
			abs, err := filepath.Abs(v)
			if err == nil {
				v = abs
			}
		}
		v = filepath.Clean(v)
		if isDir(v) {
			return v, nil
		}
		fmt.Printf("%s\n", c(ansiYellow, "Invalid directory: ")+v)
	}
	return "", errors.New("invalid scan root")
}

func expandUserHome(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			if p == "~" {
				return home
			}
			return filepath.Join(home, strings.TrimPrefix(p, "~/"))
		}
	}
	return p
}

func isDir(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" {
		return false
	}
	info, err := os.Stat(p)
	if err != nil {
		return false
	}
	return info.IsDir()
}
