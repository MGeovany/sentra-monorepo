package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mgeovany/sentra/internal/index"
	"github.com/mgeovany/sentra/internal/scanner"
)

func runAdd(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: sentra add . | sentra add <path>")
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	scanRoot := filepath.Join(homeDir, "dev")

	projects, err := scanner.Scan(scanRoot)
	if err != nil {
		return err
	}

	available := flattenScan(scanRoot, projects)

	indexPath, err := index.DefaultPath()
	if err != nil {
		return err
	}

	idx, _, err := index.Load(indexPath)
	if err != nil {
		return err
	}
	idx.ScanRoot = scanRoot
	if idx.Staged == nil {
		idx.Staged = map[string]string{}
	}

	switch args[0] {
	case ".":
		if len(available) == 0 {
			fmt.Println("✔ staged 0 env files")
			return nil
		}
		paths := make([]string, 0, len(available))
		for p := range available {
			paths = append(paths, p)
		}
		sort.Strings(paths)

		for _, p := range paths {
			idx.Staged[p] = available[p]
		}
		if err := index.Save(indexPath, idx); err != nil {
			return err
		}
		fmt.Printf("✔ staged %d env files\n", len(paths))
		return nil
	default:
		if len(args) != 1 {
			return errors.New("usage: sentra add . | sentra add <path>")
		}

		requested := normalizeRelPath(args[0])
		hash, ok := available[requested]
		if !ok {
			// allow `./foo/bar` too
			requested = strings.TrimPrefix(requested, "./")
			hash, ok = available[requested]
			if !ok {
				return fmt.Errorf("env file not found: %s", args[0])
			}
		}

		idx.Staged[requested] = hash
		if err := index.Save(indexPath, idx); err != nil {
			return err
		}
		fmt.Printf("✔ staged %s\n", requested)
		return nil
	}
}

func flattenScan(scanRoot string, projects []scanner.Project) map[string]string {
	out := make(map[string]string)
	for _, p := range projects {
		relProjectRoot, err := filepath.Rel(scanRoot, p.RootPath)
		if err != nil {
			continue
		}
		relProjectRoot = filepath.ToSlash(strings.TrimPrefix(relProjectRoot, "./"))
		for _, f := range p.EnvFiles {
			full := filepath.ToSlash(filepath.Join(relProjectRoot, f.Path))
			out[full] = f.Hash
		}
	}
	return out
}

func normalizeRelPath(p string) string {
	p = filepath.Clean(p)
	p = filepath.ToSlash(p)
	p = strings.TrimPrefix(p, "/")
	return p
}
