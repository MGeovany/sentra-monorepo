package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mgeovany/sentra/cli/internal/index"
	"github.com/mgeovany/sentra/cli/internal/scanner"
	"github.com/mgeovany/sentra/cli/internal/state"
)

type projectOverview struct {
	Root         string
	EnvCount     int
	TrackedCount int
	StagedCount  int
	ChangedCount int
	LatestFile   string
	LatestAt     time.Time
	TotalBytes   int64
}

func runOverview(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: sentra overview")
	}

	scanRoot, err := resolveScanRoot()
	if err != nil {
		return err
	}

	sp := startSpinner("Building project overview...")
	projects, err := scanner.Scan(scanRoot)
	if err != nil {
		sp.StopInfo("")
		return err
	}

	idxPath, _ := index.DefaultPath()
	idx, _, _ := index.Load(idxPath)

	statePath, _ := state.DefaultPath()
	prev, _, _ := state.Load(statePath)

	items, err := buildProjectOverviews(scanRoot, projects, idx, prev)
	if err != nil {
		sp.StopInfo("")
		return err
	}
	sp.StopSuccess(fmt.Sprintf("✔ %d project(s)", len(items)))

	if len(items) == 0 {
		infof("No git repos found under %s", scanRoot)
		return nil
	}

	printOverviewHeader(scanRoot)
	for i, it := range items {
		if i != 0 {
			fmt.Println()
		}
		printProjectCard(it)
	}
	return nil
}

func buildProjectOverviews(scanRoot string, projects []scanner.Project, idx index.Index, prev state.State) ([]projectOverview, error) {
	out := make([]projectOverview, 0, len(projects))
	for _, p := range projects {
		relRoot, err := filepath.Rel(scanRoot, p.RootPath)
		if err != nil {
			return nil, err
		}
		relRoot = filepath.ToSlash(strings.TrimPrefix(relRoot, "./"))
		relRoot = strings.TrimPrefix(relRoot, "/")
		if strings.TrimSpace(relRoot) == "" {
			continue
		}

		var latestAt time.Time
		latestFile := ""
		var totalBytes int64
		for _, f := range p.EnvFiles {
			abs := filepath.Join(p.RootPath, filepath.FromSlash(f.Path))
			st, err := os.Stat(abs)
			if err != nil {
				continue
			}
			totalBytes += st.Size()
			mt := st.ModTime()
			if latestAt.IsZero() || mt.After(latestAt) {
				latestAt = mt
				latestFile = f.Path
			}
		}

		tracked := 0
		if prev.Projects != nil {
			if m, ok := prev.Projects[relRoot]; ok {
				tracked = len(m)
			}
		}

		staged := 0
		if idx.Staged != nil {
			prefix := relRoot + "/"
			for k := range idx.Staged {
				if k == relRoot || strings.HasPrefix(k, prefix) {
					staged++
				}
			}
		}

		changed := changedCountForProject(relRoot, p.EnvFiles, prev)

		out = append(out, projectOverview{
			Root:         relRoot,
			EnvCount:     len(p.EnvFiles),
			TrackedCount: tracked,
			StagedCount:  staged,
			ChangedCount: changed,
			LatestFile:   latestFile,
			LatestAt:     latestAt,
			TotalBytes:   totalBytes,
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Root < out[j].Root })
	return out, nil
}

func changedCountForProject(relRoot string, envs []scanner.EnvFile, prev state.State) int {
	prevMap := map[string]string{}
	if prev.Projects != nil {
		if m, ok := prev.Projects[relRoot]; ok && m != nil {
			prevMap = m
		}
	}

	currMap := make(map[string]string)
	for _, f := range envs {
		currMap[f.Path] = f.Hash
	}

	changed := 0
	seen := map[string]struct{}{}
	for path, prevHash := range prevMap {
		seen[path] = struct{}{}
		currHash, ok := currMap[path]
		if !ok {
			changed++
			continue
		}
		if currHash != prevHash {
			changed++
		}
	}
	for path := range currMap {
		if _, ok := seen[path]; ok {
			continue
		}
		changed++
	}

	return changed
}

func printOverviewHeader(scanRoot string) {
	fmt.Println(c(ansiBoldCyan, "Project Overview"))
	fmt.Println(c(ansiDim, "Scan root: ") + scanRoot)
	fmt.Println(c(ansiDim, "Tip: use `sentra scan` to list files, `sentra status` for global changes"))
}

func printProjectCard(p projectOverview) {
	inner := 72
	border := "+" + strings.Repeat("-", inner) + "+"
	fmt.Println(c(ansiDim, border))
	fmt.Println(cardLine(inner, c(ansiBoldCyan, p.Root)))

	line1 := fmt.Sprintf("env: %d  tracked: %d  staged: %d", p.EnvCount, p.TrackedCount, p.StagedCount)
	line2 := fmt.Sprintf("changed: %d", p.ChangedCount)
	if p.ChangedCount == 0 {
		line2 = c(ansiGreen, line2)
	} else {
		line2 = c(ansiYellow, line2)
	}
	fmt.Println(cardLine(inner, c(ansiDim, line1+"  ")+line2))

	latest := "latest: -"
	if strings.TrimSpace(p.LatestFile) != "" && !p.LatestAt.IsZero() {
		latest = fmt.Sprintf("latest: %s  (%s)", p.LatestFile, p.LatestAt.Format("2006-01-02 15:04"))
	}
	fmt.Println(cardLine(inner, c(ansiDim, latest)))

	if p.TotalBytes > 0 {
		fmt.Println(cardLine(inner, c(ansiDim, fmt.Sprintf("size: %s", formatBytes(p.TotalBytes)))))
	}

	fmt.Println(c(ansiDim, border))
}

func cardLine(inner int, content string) string {
	// Leave a single leading/trailing space inside the box.
	// content is assumed to be short; if it's long, truncate without trying to account for ANSI width.
	plain := stripANSICodes(content)
	max := inner - 2
	if len(plain) > max {
		// naive truncate; good enough for our short lines
		plain = plain[:max-3] + "..."
		content = plain
	}
	pad := ""
	if n := max - len(stripANSICodes(content)); n > 0 {
		pad = strings.Repeat(" ", n)
	}
	return "| " + content + pad + " |"
}

func stripANSICodes(s string) string {
	// Minimal ANSI stripper for our own sequences.
	// This is not a full parser, but enough to keep padding stable.
	out := make([]rune, 0, len(s))
	inEsc := false
	for i := 0; i < len(s); i++ {
		b := s[i]
		if inEsc {
			if b == 'm' {
				inEsc = false
			}
			continue
		}
		if b == 0x1b {
			// expect "[...m"
			if i+1 < len(s) && s[i+1] == '[' {
				inEsc = true
				continue
			}
		}
		out = append(out, rune(b))
	}
	return string(out)
}

func formatBytes(n int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
	)
	if n < kb {
		return fmt.Sprintf("%d B", n)
	}
	if n < mb {
		return fmt.Sprintf("%.1f KB", float64(n)/kb)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/mb)
}
