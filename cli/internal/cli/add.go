package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mgeovany/sentra/cli/internal/index"
	"github.com/mgeovany/sentra/cli/internal/scanner"
)

func runAdd(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: sentra add . | sentra add <path>")
	}

	verbosef("Starting add operation...")
	// scan root is configurable and persisted in local index
	scanRoot, err := resolveScanRoot()
	if err != nil {
		return err
	}
	verbosef("Scan root: %s", scanRoot)

	projects, err := scanner.Scan(scanRoot)
	if err != nil {
		return err
	}
	verbosef("Scanned %d project(s)", len(projects))

	available := flattenScan(scanRoot, projects)
	verbosef("Found %d available env file(s)", len(available))

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
			fmt.Println(c(ansiGreen, "✔ staged ") + c(ansiBoldCyan, "0") + c(ansiGreen, " env files"))
			verbosef("No env files found to stage")
			return nil
		}
		paths := make([]string, 0, len(available))
		for p := range available {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		verbosef("Staging %d file(s):", len(paths))
		for _, p := range paths {
			verbosef("  - %s (hash: %s)", p, available[p])
		}

		for _, p := range paths {
			idx.Staged[p] = available[p]
		}
		if err := index.Save(indexPath, idx); err != nil {
			return err
		}
		fmt.Println(c(ansiGreen, "✔ staged ") + c(ansiBoldCyan, fmt.Sprintf("%d", len(paths))) + c(ansiGreen, " env files"))
		verbosef("Index saved to: %s", indexPath)
		return nil
	default:
		if len(args) != 1 {
			return errors.New("usage: sentra add . | sentra add <path>")
		}

		requested := normalizeRelPath(args[0])
		verbosef("Looking for file: %s", requested)
		hash, ok := available[requested]
		if !ok {
			// allow `./foo/bar` too
			requested = strings.TrimPrefix(requested, "./")
			verbosef("Trying without ./ prefix: %s", requested)
			hash, ok = available[requested]
			if !ok {
				verbosef("File not found in available files")
				return fmt.Errorf("env file not found: %s", args[0])
			}
		}
		verbosef("Found file: %s (hash: %s)", requested, hash)

		idx.Staged[requested] = hash
		if err := index.Save(indexPath, idx); err != nil {
			return err
		}
		fmt.Println(c(ansiGreen, "✔ staged ") + c(ansiBoldCyan, requested))
		verbosef("Index saved to: %s", indexPath)
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
