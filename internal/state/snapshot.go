package state

import (
	"path/filepath"

	"github.com/mgeovany/sentra/internal/scanner"
)

func FromScan(scanRoot string, projects []scanner.Project) (State, error) {
	out := State{
		ScanRoot: scanRoot,
		Projects: map[string]map[string]string{},
		Version:  1,
	}

	for _, p := range projects {
		relProjectRoot, err := filepath.Rel(scanRoot, p.RootPath)
		if err != nil {
			return State{}, err
		}
		relProjectRoot = filepath.ToSlash(relProjectRoot)

		envs := make(map[string]string)
		for _, f := range p.EnvFiles {
			envs[f.Path] = f.Hash
		}
		out.Projects[relProjectRoot] = envs
	}

	return out, nil
}
