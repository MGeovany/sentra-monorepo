package cli

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/mgeovany/sentra/internal/commit"
)

func runLog() error {
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
		fmt.Printf("commit %s\n", shortCommitID(c))

		date := formatCommitDate(c.CreatedAt)
		if date != "" {
			fmt.Printf("Date: %s\n", date)
		}

		fmt.Println("Message:")
		fmt.Printf("  %s\n", strings.TrimSpace(c.Message))
		if i != 0 {
			fmt.Println()
		}
	}

	return nil
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
