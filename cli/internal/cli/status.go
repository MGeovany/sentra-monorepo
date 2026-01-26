package cli

import (
	"fmt"

	"github.com/mgeovany/sentra/cli/internal/scanner"
	"github.com/mgeovany/sentra/cli/internal/state"
)

func runStatus() error {
	scanRoot, err := resolveScanRoot()
	if err != nil {
		return err
	}

	statePath, err := state.DefaultPath()
	if err != nil {
		return err
	}

	prev, ok, err := state.Load(statePath)
	if err != nil {
		return err
	}
	if !ok {
		prev = state.State{ScanRoot: scanRoot, Projects: map[string]map[string]string{}, Version: 1}
	}

	currentProjects, err := scanner.Scan(scanRoot)
	if err != nil {
		return err
	}
	curr, err := state.FromScan(scanRoot, currentProjects)
	if err != nil {
		return err
	}

	changed := countChangedEnvFiles(prev, curr)

	fmt.Printf("✔ %d projects tracked\n", len(prev.Projects))
	fmt.Printf("⚠ %d env changed\n", changed)

	return nil
}

func countChangedEnvFiles(prev state.State, curr state.State) int {
	prevFiles := flattenEnv(prev)
	currFiles := flattenEnv(curr)

	changed := 0
	seen := map[string]struct{}{}

	for k, prevHash := range prevFiles {
		seen[k] = struct{}{}
		currHash, ok := currFiles[k]
		if !ok {
			changed++
			continue
		}
		if currHash != prevHash {
			changed++
		}
	}

	for k := range currFiles {
		if _, ok := seen[k]; ok {
			continue
		}
		// new file
		changed++
	}

	return changed
}

func flattenEnv(s state.State) map[string]string {
	out := make(map[string]string)
	for projectRoot, envs := range s.Projects {
		for envPath, hash := range envs {
			key := projectRoot + ":" + envPath
			out[key] = hash
		}
	}
	return out
}
