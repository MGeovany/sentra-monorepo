package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mgeovany/sentra/internal/commit"
	"github.com/mgeovany/sentra/internal/index"
)

func runCommit(args []string) error {
	message, err := parseCommitMessage(args)
	if err != nil {
		return err
	}

	indexPath, err := index.DefaultPath()
	if err != nil {
		return err
	}

	idx, ok, err := index.Load(indexPath)
	if err != nil {
		return err
	}
	if !ok || len(idx.Staged) == 0 {
		return errors.New("nothing to commit (no staged env files)")
	}

	c := commit.New(message, idx.Staged)
	if _, err := commit.Save(c); err != nil {
		return err
	}

	idx.Staged = map[string]string{}
	if err := index.Save(indexPath, idx); err != nil {
		return err
	}

	fmt.Printf("✔ committed %s\n", c.ID)
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
