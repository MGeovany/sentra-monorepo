package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mgeovany/sentra/cli/internal/commit"
	"github.com/mgeovany/sentra/cli/internal/index"
)

func runCommit(args []string) error {
	verbosef("Starting commit operation...")
	message, err := parseCommitMessage(args)
	if err != nil {
		return err
	}
	verbosef("Commit message: %s", message)

	indexPath, err := index.DefaultPath()
	if err != nil {
		return err
	}
	verbosef("Index path: %s", indexPath)

	idx, ok, err := index.Load(indexPath)
	if err != nil {
		return err
	}
	if !ok || len(idx.Staged) == 0 {
		return errors.New("nothing to commit (no staged env files)")
	}
	verbosef("Found %d staged file(s)", len(idx.Staged))
	for path, hash := range idx.Staged {
		verbosef("  - %s (hash: %s)", path, hash)
	}

	// Run fmt and lint before committing
	if err := runPreCommitChecks(); err != nil {
		return fmt.Errorf("pre-commit checks failed: %w", err)
	}

	cm := commit.New(message, idx.Staged)
	verbosef("Created commit: %s", cm.ID)
	if _, err := commit.Save(cm); err != nil {
		return err
	}
	verbosef("Commit saved to local storage")

	idx.Staged = map[string]string{}
	if err := index.Save(indexPath, idx); err != nil {
		return err
	}
	verbosef("Index cleared and saved")

	// Keep this one a bit more celebratory.
	shortID := cm.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	fmt.Println(c(ansiGreen, "✔ committed ") + c(ansiBoldCyan, shortID))
	verbosef("Commit %s created with %d file(s)", cm.ID, len(cm.Files))
	return nil
}

func parseCommitMessage(args []string) (string, error) {
	if len(args) < 2 {
		return "", errors.New("usage: sentra commit -m 'message'")
	}

	var message string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-m":
			if i+1 >= len(args) {
				return "", errors.New("usage: sentra commit -m 'message'")
			}
			message = args[i+1]
			i++
		default:
			return "", errors.New("usage: sentra commit -m 'message'")
		}
	}

	message = strings.TrimSpace(message)
	if message == "" {
		return "", errors.New("commit message cannot be empty")
	}

	return message, nil
}

func runPreCommitChecks() error {
	// Find the project root (monorepo root)
	projectRoot, err := findProjectRoot()
	if err != nil {
		// If we can't find the root, warn but don't fail (might be running outside repo)
		warnf("⚠ Could not find project root, skipping pre-commit checks: %v", err)
		verbosef("Current working directory: %s", func() string {
			wd, _ := os.Getwd()
			return wd
		}())
		return nil
	}
	verbosef("Project root: %s", projectRoot)

	// Run go fmt and verify
	sp := startSpinner("Formatting and verifying code...")
	if err := runGoFmt(projectRoot); err != nil {
		sp.StopInfo("")
		warnf("⚠ Pre-commit check failed:")
		fmt.Println()
		fmt.Println(err.Error())
		fmt.Println()
		return fmt.Errorf("code formatting check failed - please fix formatting issues before committing")
	}
	sp.StopSuccess("✔ Code formatted and verified")

	// Run lint
	sp2 := startSpinner("Linting code...")
	if err := runLint(projectRoot); err != nil {
		sp2.StopInfo("")
		return fmt.Errorf("lint failed: %w", err)
	}
	sp2.StopSuccess("✔ Lint passed")

	return nil
}

func findProjectRoot() (string, error) {
	// Start from the current working directory
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Look for go.work or go.mod in the root
	for {
		// Check for go.work (monorepo indicator) or go.mod at root level
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir, nil
		}
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			// Check if we're in a subdirectory (cli/ or server/)
			parent := filepath.Dir(dir)
			if _, err := os.Stat(filepath.Join(parent, "go.work")); err == nil {
				return parent, nil
			}
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("could not find project root")
}

func runGoFmt(projectRoot string) error {
	// First, format the code
	cliDir := filepath.Join(projectRoot, "cli")
	if _, err := os.Stat(cliDir); err == nil {
		cmd := exec.Command("go", "fmt", "./...")
		cmd.Dir = cliDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("go fmt in cli/ failed: %w", err)
		}
		verbosef("Formatted CLI code")
	}

	serverDir := filepath.Join(projectRoot, "server")
	if _, err := os.Stat(serverDir); err == nil {
		cmd := exec.Command("go", "fmt", "./...")
		cmd.Dir = serverDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("go fmt in server/ failed: %w", err)
		}
		verbosef("Formatted Server code")
	}

	// Then verify that all files are properly formatted
	if err := verifyGoFmt(projectRoot); err != nil {
		return err
	}

	return nil
}

func verifyGoFmt(projectRoot string) error {
	// Check CLI code
	cliDir := filepath.Join(projectRoot, "cli")
	if _, err := os.Stat(cliDir); err == nil {
		// Use gofmt -l to find unformatted files
		cmd := exec.Command("gofmt", "-l", ".")
		cmd.Dir = cliDir
		unformatted, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("gofmt check failed in cli/: %w (gofmt is required for pre-commit checks)", err)
		}
		unformattedStr := strings.TrimSpace(string(unformatted))
		if unformattedStr != "" {
			lines := strings.Split(unformattedStr, "\n")
			var fileList []string
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" {
					fileList = append(fileList, filepath.Join("cli", line))
				}
			}
			return fmt.Errorf("files need formatting:\n%s\nRun: go fmt ./... in cli/", strings.Join(fileList, "\n"))
		}
		verbosef("CLI code formatting verified")
	}

	// Check Server code
	serverDir := filepath.Join(projectRoot, "server")
	if _, err := os.Stat(serverDir); err == nil {
		// Use gofmt -l to find unformatted files
		cmd := exec.Command("gofmt", "-l", ".")
		cmd.Dir = serverDir
		unformatted, err := cmd.Output()
		if err != nil {
			return fmt.Errorf("gofmt check failed in server/: %w (gofmt is required for pre-commit checks)", err)
		}
		unformattedStr := strings.TrimSpace(string(unformatted))
		if unformattedStr != "" {
			lines := strings.Split(unformattedStr, "\n")
			var fileList []string
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" {
					fileList = append(fileList, filepath.Join("server", line))
				}
			}
			return fmt.Errorf("files need formatting:\n%s\nRun: go fmt ./... in server/", strings.Join(fileList, "\n"))
		}
		verbosef("Server code formatting verified")
	}

	return nil
}

func runLint(projectRoot string) error {
	// Find golangci-lint
	golangciLint, err := findGolangciLint()
	if err != nil {
		verbosef("golangci-lint not found, skipping lint: %v", err)
		return nil // Don't fail if lint tool is not available
	}

	// Lint CLI code
	cliDir := filepath.Join(projectRoot, "cli")
	if _, err := os.Stat(cliDir); err == nil {
		cmd := exec.Command(golangciLint, "run", "./...")
		cmd.Dir = cliDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("lint in cli/ failed: %w", err)
		}
		verbosef("Linted CLI code")
	}

	// Lint Server code
	serverDir := filepath.Join(projectRoot, "server")
	if _, err := os.Stat(serverDir); err == nil {
		cmd := exec.Command(golangciLint, "run", "./...")
		cmd.Dir = serverDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("lint in server/ failed: %w", err)
		}
		verbosef("Linted Server code")
	}

	return nil
}

func findGolangciLint() (string, error) {
	// Check if it's in PATH
	if path, err := exec.LookPath("golangci-lint"); err == nil {
		return path, nil
	}

	// Check GOPATH/bin
	if gopath := os.Getenv("GOPATH"); gopath != "" {
		candidate := filepath.Join(gopath, "bin", "golangci-lint")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	// Try to get GOPATH from go env
	cmd := exec.Command("go", "env", "GOPATH")
	output, err := cmd.Output()
	if err == nil {
		gopath := strings.TrimSpace(string(output))
		if gopath != "" {
			candidate := filepath.Join(gopath, "bin", "golangci-lint")
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		}
	}

	return "", fmt.Errorf("golangci-lint not found in PATH or GOPATH/bin")
}
