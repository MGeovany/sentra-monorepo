package cli

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mgeovany/sentra/cli/internal/commit"
	"github.com/mgeovany/sentra/cli/internal/index"
)

func runLog(args []string) error {
	if len(args) == 0 {
		return runLogList()
	}

	switch strings.TrimSpace(args[0]) {
	case "rm", "delete":
		if len(args) != 2 {
			return errors.New("usage: sentra log rm <id>")
		}
		return runLogDelete(strings.TrimSpace(args[1]))
	case "clear":
		if len(args) != 1 {
			return errors.New("usage: sentra log clear")
		}
		return runLogClear()
	case "prune":
		if len(args) != 2 {
			return errors.New("usage: sentra log prune <id|all>")
		}
		return runLogPrune(strings.TrimSpace(args[1]))
	case "verify":
		if len(args) != 1 {
			return errors.New("usage: sentra log verify")
		}
		return runLogVerify()
	default:
		return errors.New("usage: sentra log [rm <id>|clear|prune <id|all>|verify]")
	}
}

func runLogList() error {
	commits, err := commit.List()
	if err != nil {
		return err
	}

	if len(commits) == 0 {
		fmt.Println("no commits")
		return nil
	}

	// newest first
	for i := len(commits) - 1; i >= 0; i-- {
		c := commits[i]
		fmt.Printf("commit %s (%s)\n", shortCommitID(c), c.ID)

		date := formatCommitDate(c.CreatedAt)
		if date != "" {
			fmt.Printf("Date: %s\n", date)
		}
		if strings.TrimSpace(c.PushedAt) != "" {
			fmt.Printf("Pushed: %s\n", strings.TrimSpace(c.PushedAt))
		}
		fmt.Printf("Files: %d\n", len(c.Files))

		msg := strings.TrimSpace(c.Message)
		if msg == "" {
			msg = "(no message)"
		}
		fmt.Println("Message:")
		fmt.Printf("  %s\n", msg)
		if i != 0 {
			fmt.Println()
		}
	}

	return nil
}

func runLogDelete(selector string) error {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return errors.New("usage: sentra log rm <id>")
	}

	commits, err := commit.List()
	if err != nil {
		return err
	}
	if len(commits) == 0 {
		return errors.New("no commits")
	}

	id, err := resolveCommitID(commits, selector)
	if err != nil {
		return err
	}

	if err := commit.Delete(id); err != nil {
		return err
	}
	fmt.Printf("✔ deleted commit %s\n", id)
	return nil
}

func runLogClear() error {
	n, err := commit.Clear()
	if err != nil {
		return err
	}
	fmt.Printf("✔ deleted %d commits\n", n)
	return nil
}

func runLogVerify() error {
	scanRoot, err := resolveScanRootFromIndex()
	if err != nil {
		return err
	}

	commits, err := commit.List()
	if err != nil {
		return err
	}
	if len(commits) == 0 {
		fmt.Println("no commits")
		return nil
	}

	issues := 0
	for _, c := range commits {
		if strings.TrimSpace(c.PushedAt) != "" {
			continue
		}
		missing := missingFilesForCommit(scanRoot, c)
		if len(missing) == 0 {
			continue
		}
		issues++
		fmt.Printf("commit %s missing %d file(s):\n", c.ID, len(missing))
		for _, p := range missing {
			fmt.Printf("  %s\n", p)
		}
		fmt.Printf("  fix: sentra log prune %s\n\n", c.ID)
	}

	if issues == 0 {
		fmt.Println("✔ all pending commits are readable")
		return nil
	}
	return errors.New("missing files detected in pending commits")
}

func runLogPrune(selector string) error {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return errors.New("usage: sentra log prune <id|all>")
	}

	scanRoot, err := resolveScanRootFromIndex()
	if err != nil {
		return err
	}

	commits, err := commit.List()
	if err != nil {
		return err
	}
	if len(commits) == 0 {
		return errors.New("no commits")
	}

	var targets []commit.Commit
	if selector == "all" {
		for _, c := range commits {
			if strings.TrimSpace(c.PushedAt) != "" {
				continue
			}
			targets = append(targets, c)
		}
	} else {
		id, err := resolveCommitID(commits, selector)
		if err != nil {
			return err
		}
		for _, c := range commits {
			if c.ID == id {
				targets = append(targets, c)
				break
			}
		}
	}

	if len(targets) == 0 {
		fmt.Println("✔ nothing to prune")
		return nil
	}

	prunedCommits := 0
	prunedFiles := 0
	deletedCommits := 0
	for _, c := range targets {
		missing := missingFilesForCommit(scanRoot, c)
		if len(missing) == 0 {
			continue
		}
		for _, p := range missing {
			delete(c.Files, p)
		}
		prunedFiles += len(missing)
		prunedCommits++

		if len(c.Files) == 0 {
			if err := commit.Delete(c.ID); err != nil {
				return err
			}
			deletedCommits++
			continue
		}
		if err := commit.Update(c); err != nil {
			return err
		}
	}

	if prunedCommits == 0 {
		fmt.Println("✔ nothing to prune")
		return nil
	}
	if deletedCommits > 0 {
		fmt.Printf("✔ pruned %d missing file(s) across %d commit(s) (%d commit(s) deleted)\n", prunedFiles, prunedCommits, deletedCommits)
		return nil
	}
	fmt.Printf("✔ pruned %d missing file(s) across %d commit(s)\n", prunedFiles, prunedCommits)
	return nil
}

func resolveScanRootFromIndex() (string, error) {
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
			return v, nil
		}
	}
	return defaultRoot, nil
}

func missingFilesForCommit(scanRoot string, c commit.Commit) []string {
	var missing []string
	for p := range c.Files {
		abs := filepath.Join(scanRoot, filepath.FromSlash(p))
		if _, err := os.Stat(abs); err != nil {
			if os.IsNotExist(err) {
				missing = append(missing, p)
			}
		}
	}
	return missing
}

func resolveCommitID(commits []commit.Commit, selector string) (string, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return "", errors.New("commit id is required")
	}
	if _, err := uuid.Parse(selector); err == nil {
		for _, c := range commits {
			if c.ID == selector {
				return c.ID, nil
			}
		}
		return "", fmt.Errorf("commit not found: %s", selector)
	}

	var matches []commit.Commit
	for _, c := range commits {
		if shortCommitID(c) == selector {
			matches = append(matches, c)
			continue
		}
		if strings.HasPrefix(c.ID, selector) {
			matches = append(matches, c)
			continue
		}
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("commit not found: %s", selector)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("commit selector is ambiguous: %s", selector)
	}
	return matches[0].ID, nil
}

func shortCommitID(c commit.Commit) string {
	// Git-like short id, stable and noise-free.
	sum := sha1.Sum([]byte(c.ID + "\n" + c.CreatedAt + "\n" + c.Message))
	return hex.EncodeToString(sum[:])[:6]
}

func formatCommitDate(createdAt string) string {
	if createdAt == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return ""
	}
	return t.Format("2006-01-02")
}
