package cli

import (
	"fmt"

	"github.com/mgeovany/sentra/cli/internal/scanner"
	"github.com/mgeovany/sentra/cli/internal/state"
)

func runStatus() error {
	verbosef("Checking status...")
	scanRoot, err := resolveScanRoot()
	if err != nil {
		return err
	}
	verbosef("Scan root: %s", scanRoot)

	statePath, err := state.DefaultPath()
	if err != nil {
		return err
	}
	verbosef("State path: %s", statePath)

	prev, ok, err := state.Load(statePath)
	if err != nil {
		return err
	}
	if !ok {
		prev = state.State{ScanRoot: scanRoot, Projects: map[string]map[string]string{}, Version: 1}
		verbosef("No previous state found, starting fresh")
	} else {
		verbosef("Loaded previous state with %d project(s)", len(prev.Projects))
	}

	verbosef("Scanning current state...")
	currentProjects, err := scanner.Scan(scanRoot)
	if err != nil {
		return err
	}
	verbosef("Found %d current project(s)", len(currentProjects))
	curr, err := state.FromScan(scanRoot, currentProjects)
	if err != nil {
		return err
	}
	verbosef("Current state has %d project(s)", len(curr.Projects))

	changed := countChangedEnvFiles(prev, curr)
	verbosef("Changed files detected: %d", changed)

	fmt.Println(c(ansiGreen, "✔ ") + c(ansiBoldCyan, fmt.Sprintf("%d", len(prev.Projects))) + c(ansiGreen, " projects tracked"))
	if changed == 0 {
		fmt.Println(c(ansiGreen, "✔ ") + c(ansiBoldCyan, "0") + c(ansiGreen, " env changed"))
		verbosef("All env files are up to date")
		return nil
	}
	fmt.Println(c(ansiYellow, "⚠ ") + c(ansiBoldCyan, fmt.Sprintf("%d", changed)) + c(ansiYellow, " env changed"))
	verbosef("Run 'sentra add .' to stage changed files")

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
