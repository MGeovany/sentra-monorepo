package cli

import (
	"fmt"
	"sort"
	"time"

	"github.com/mgeovany/sentra/cli/internal/commit"
)

func runPush() error {
	if err := ensureRemoteSession(); err != nil {
		return err
	}

	commits, err := commit.List()
	if err != nil {
		return err
	}

	var pending []commit.Commit
	for _, c := range commits {
		if c.PushedAt == "" {
			pending = append(pending, c)
		}
	}

	if len(pending) == 0 {
		fmt.Println("✔ nothing to push")
		return nil
	}

	sort.Slice(pending, func(i, j int) bool { return pending[i].ID < pending[j].ID })

	now := time.Now().UTC().Format(time.RFC3339)
	for _, c := range pending {
		c.PushedAt = now
		if err := commit.Update(c); err != nil {
			return err
		}
		fmt.Printf("✔ pushed commit %s\n", c.ID)
	}

	return nil
}
