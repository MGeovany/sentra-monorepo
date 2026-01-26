package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type remoteProject struct {
	RootPath          string `json:"root_path"`
	LastCommitID      string `json:"last_commit_id"`
	LastCommitMessage string `json:"last_commit_message"`
	FileCount         int    `json:"file_count"`
}

func runProjects() error {
	verbosef("Fetching projects from remote...")
	sess, err := ensureRemoteSession()
	if err != nil {
		return err
	}
	if strings.TrimSpace(sess.AccessToken) == "" {
		return fmt.Errorf("not logged in (run: sentra login)")
	}
	verbosef("Session loaded: user authenticated")

	sp := startSpinner("Fetching projects from remote...")

	serverURL, err := serverURLFromEnv()
	if err != nil {
		sp.StopInfo("")
		return err
	}
	verbosef("Server URL: %s", serverURL)

	endpoint := serverURL + "/projects"
	verbosef("Projects endpoint: %s", endpoint)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(sess.AccessToken))

	client := &http.Client{Timeout: 15 * time.Second}
	startTime := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(startTime)
	if err != nil {
		sp.StopInfo("")
		verbosef("Request failed after %v: %v", elapsed, err)
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	verbosef("Response received: status=%d, elapsed=%v", resp.StatusCode, elapsed)

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		sp.StopInfo("")
		verbosef("Response body: %s", string(body))
		return fmt.Errorf("failed to fetch projects")
	}
	verbosef("Response body size: %d bytes", len(body))

	var projects []remoteProject
	if err := json.Unmarshal(body, &projects); err != nil {
		sp.StopInfo("")
		return err
	}
	sp.StopSuccess(fmt.Sprintf("✔ %d project(s)", len(projects)))
	verbosef("Parsed %d project(s) from response", len(projects))

	if len(projects) == 0 {
		// spinner already printed count
		return nil
	}

	for _, p := range projects {
		root := strings.TrimSpace(p.RootPath)
		if root == "" {
			root = "(unknown)"
		}

		last := strings.TrimSpace(p.LastCommitID)
		if last == "" {
			last = "-"
		} else if len(last) > 8 {
			last = last[:8]
		}

		msg := strings.TrimSpace(p.LastCommitMessage)
		if isVerbose() {
			verbosef("Project: %s, commit: %s, files: %d, message: %s", root, last, p.FileCount, msg)
		}
		if msg != "" {
			fmt.Printf("%s\t%s\t%d\t%s\n", root, last, p.FileCount, msg)
			continue
		}
		fmt.Printf("%s\t%s\t%d\n", root, last, p.FileCount)
	}

	return nil
}
