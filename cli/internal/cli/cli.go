package cli

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mgeovany/sentra/cli/internal/scanner"
)

func Execute(args []string) error {
	if len(args) == 0 {
		return usageError()
	}

	switch args[0] {
	case "login":
		if len(args) > 1 {
			return errors.New("sentra login does not accept flags/args yet")
		}
		return runLogin()
	case "storage":
		return runStorage(args[1:])
	case "export":
		return runExport(args[1:])
	case "files":
		return runFiles(args[1:])
	case "commits":
		return runCommits(args[1:])
	case "projects":
		if len(args) > 1 {
			return errors.New("sentra projects does not accept flags/args yet")
		}
		return runProjects()
	case "who":
		if len(args) > 1 {
			return errors.New("sentra who does not accept flags/args yet")
		}
		return runWho()
	case "scan":

		if len(args) > 1 {
			return errors.New("sentra scan does not accept flags/args yet")
		}
		return runScan()
	case "add":
		return runAdd(args[1:])
	case "status":
		if len(args) > 1 {
			return errors.New("sentra status does not accept flags/args yet")
		}
		return runStatus()
	case "commit":
		return runCommit(args[1:])
	case "sync":
		return runSync(args[1:])
	case "log":
		return runLog(args[1:])
	case "push":
		if len(args) > 1 {
			return errors.New("sentra push does not accept flags/args yet")
		}
		return runPush()
	case "wipe":
		return runWipe(args[1:])
	case "doctor":
		if len(args) > 1 {
			return errors.New("sentra doctor does not accept flags/args yet")
		}
		return runDoctor()
	default:
		return usageError()
	}
}

func usageError() error {
	return errors.New("usage: sentra login | sentra storage setup|status|test|reset | sentra projects | sentra commits <project> | sentra files <project> [--at <commit>] | sentra export <project> [--at <commit>] | sentra who | sentra scan | sentra add | sentra status | sentra commit | sentra sync | sentra log [all|pending|pushed|rm <id>|clear|prune <id|all>|verify] | sentra push | sentra wipe | sentra doctor")
}

func runScan() error {
	scanRoot, err := resolveScanRoot()
	if err != nil {
		return err
	}

	projects, err := scanner.Scan(scanRoot)
	if err != nil {
		return err
	}

	envCount := 0
	for _, p := range projects {
		envCount += len(p.EnvFiles)
	}

	fmt.Printf("✔ %d projects found\n", len(projects))

	projectRoots := make([]string, 0, len(projects))
	for _, project := range projects {
		relProjectRoot, err := filepath.Rel(scanRoot, project.RootPath)
		if err != nil {
			return err
		}
		projectRoots = append(projectRoots, filepath.ToSlash(strings.TrimPrefix(relProjectRoot, "./")))
	}

	sort.Strings(projectRoots)
	for _, p := range projectRoots {
		fmt.Println(p)
	}

	fmt.Println()
	fmt.Printf("✔ %d env files detected\n\n", envCount)

	var lines []string
	for _, project := range projects {
		relProjectRoot, err := filepath.Rel(scanRoot, project.RootPath)
		if err != nil {
			return err
		}

		for _, envFile := range project.EnvFiles {
			fullRel := filepath.ToSlash(filepath.Join(relProjectRoot, envFile.Path))
			lines = append(lines, fullRel)
		}
	}

	sort.Strings(lines)
	for _, line := range lines {
		fmt.Println(strings.TrimPrefix(line, "./"))
	}

	return nil
}
