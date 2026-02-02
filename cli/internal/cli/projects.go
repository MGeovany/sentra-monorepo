package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
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

	// Pretty table for humans; TSV for scripts.
	if isTTY(os.Stdout) {
		printProjectsTable(projects)
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

func printProjectsTable(projects []remoteProject) {
	// Header
	fmt.Println(c(ansiBoldCyan, "sentra projects"))

	// Column sizing
	width := 80
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		width = w
	}
	if width < 60 {
		width = 60
	}

	projW := 10
	for _, p := range projects {
		root := strings.TrimSpace(p.RootPath)
		if root == "" {
			root = "(unknown)"
		}
		if len(root) > projW {
			projW = len(root)
		}
	}
	if projW < 12 {
		projW = 12
	}
	if projW > 28 {
		projW = 28
	}

	commitW := 8
	filesW := 5
	gap := 2
	minMsgW := 20
	msgW := width - projW - commitW - filesW - (gap * 3)
	if msgW < minMsgW {
		msgW = minMsgW
	}

	header := fmt.Sprintf(
		"%s%s%s%s%s%s%s",
		padRight("PROJECT", projW),
		strings.Repeat(" ", gap),
		padRight("COMMIT", commitW),
		strings.Repeat(" ", gap),
		padLeft("FILES", filesW),
		strings.Repeat(" ", gap),
		padRight("MESSAGE", msgW),
	)
	fmt.Println(c(ansiDim, header))
	fmt.Println(c(ansiDim, strings.Repeat("-", minInt(width, len(header)))))

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
		if msg == "" {
			msg = "-"
		}

		colProject := padRight(truncate(root, projW), projW)
		colCommit := padRight(truncate(last, commitW), commitW)
		colFiles := padLeft(fmt.Sprintf("%d", p.FileCount), filesW)
		colMsg := padRight(truncate(msg, msgW), msgW)

		colCommit = c(ansiCyan, colCommit)
		if p.FileCount == 0 {
			colFiles = c(ansiDim, colFiles)
		} else {
			colFiles = c(ansiBoldCyan, colFiles)
		}

		fmt.Printf(
			"%s%s%s%s%s%s%s\n",
			colProject,
			strings.Repeat(" ", gap),
			colCommit,
			strings.Repeat(" ", gap),
			colFiles,
			strings.Repeat(" ", gap),
			colMsg,
		)
	}
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

func padLeft(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return strings.Repeat(" ", n-len(s)) + s
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
