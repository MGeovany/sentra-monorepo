package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// sentra history
// Lists remote commit history across all projects.
func runHistory(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: sentra history")
	}

	sess, err := ensureRemoteSession()
	if err != nil {
		return err
	}
	if strings.TrimSpace(sess.AccessToken) == "" {
		return errors.New("not logged in (run: sentra login)")
	}

	serverURL, err := serverURLFromEnv()
	if err != nil {
		return err
	}

	projects, err := fetchRemoteProjects(serverURL, sess.AccessToken)
	if err != nil {
		return err
	}
	if len(projects) == 0 {
		fmt.Println("✔ 0 projects")
		return nil
	}

	sort.Slice(projects, func(i, j int) bool {
		return strings.TrimSpace(projects[i].RootPath) < strings.TrimSpace(projects[j].RootPath)
	})

	total := 0
	for _, p := range projects {
		root := strings.TrimSpace(p.RootPath)
		if root == "" {
			continue
		}
		commits, err := fetchRemoteCommits(serverURL, sess.AccessToken, root)
		if err != nil {
			return err
		}
		if len(commits) == 0 {
			continue
		}

		fmt.Println(root)
		for _, c := range commits {
			created := strings.TrimSpace(c.CreatedAt)
			if created == "" {
				created = "-"
			}
			id := strings.TrimSpace(c.CommitID)
			short := id
			if len(short) > 8 {
				short = short[:8]
			}
			machine := strings.TrimSpace(c.MachineName)
			if machine == "" {
				machine = strings.TrimSpace(c.MachineID)
			}
			if machine == "" {
				machine = "unknown"
			}
			msg := strings.TrimSpace(c.Message)
			if msg == "" {
				msg = "(no message)"
			}
			cnt := c.FileCount
			if cnt == 0 {
				cnt = len(c.FilePaths)
			}
			fmt.Printf("  %s\t%s\t%d\t%s\t%s\n", created, short, cnt, machine, msg)
			total++
		}
		fmt.Println()
	}

	if total == 0 {
		fmt.Println("✔ 0 commits")
		return nil
	}
	fmt.Printf("✔ %d commit(s)\n", total)
	return nil
}

func fetchRemoteCommits(serverURL string, accessToken string, root string) ([]remoteCommit, error) {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(serverURL), "/") + "/commits")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("root", strings.TrimSpace(root))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))

	client := &http.Client{Timeout: 25 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("failed to fetch commits")
	}

	var commits []remoteCommit
	if err := json.Unmarshal(body, &commits); err != nil {
		return nil, err
	}
	return commits, nil
}
