package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type remoteCommit struct {
	CommitID    string   `json:"commit_id"`
	CreatedAt   string   `json:"created_at"`
	Message     string   `json:"message"`
	MachineName string   `json:"machine_name"`
	MachineID   string   `json:"machine_id"`
	FilePaths   []string `json:"files"`
	ProjectRoot string   `json:"project_root"`
	ProjectID   string   `json:"project_id"`
	ProjectName string   `json:"project_name"`
	FileCount   int      `json:"file_count"`
}

func runCommits(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: sentra commits <project>")
	}
	
	root := projectRootFromPath(args[0])
	root = strings.TrimSpace(root)
	if root == "" {
		return errors.New("usage: sentra commits <project>")
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

	u, err := url.Parse(serverURL + "/commits")
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("root", root)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(sess.AccessToken))

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("commits failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var commits []remoteCommit
	if err := json.Unmarshal(body, &commits); err != nil {
		return err
	}
	if len(commits) == 0 {
		fmt.Println("✔ 0 commits")
		return nil
	}

	for _, c := range commits {
		machine := strings.TrimSpace(c.MachineName)
		if machine == "" {
			machine = strings.TrimSpace(c.MachineID)
		}
		if machine == "" {
			machine = "unknown"
		}

		created := strings.TrimSpace(c.CreatedAt)
		if created == "" {
			created = "-"
		}

		msg := strings.TrimSpace(c.Message)
		if msg == "" {
			msg = "(no message)"
		}

		fmt.Printf("%s\t%s\t%s\n", created, machine, msg)
		for _, p := range c.FilePaths {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			fmt.Printf("  %s\n", p)
		}
		fmt.Println()
	}

	return nil
}
