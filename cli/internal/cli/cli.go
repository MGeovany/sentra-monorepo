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

	// Global flags / commands.
	switch args[0] {
	case "version", "--version", "-v", "-V":
		if len(args) > 1 {
			return errors.New("sentra version does not accept flags/args")
		}
		printVersion()
		return nil
	case "help", "--help", "-h":
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
	case "history":
		return runHistory(args[1:])
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
	case "overview":
		return runOverview(args[1:])
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
	return errors.New(strings.TrimSpace(`sentra

Usage:
  sentra <command> [args]

Global:
  sentra version | --version
  sentra help | --help | -h

Auth:
  sentra login              Login and create a session
  sentra who                Show current logged-in user

Remote (cloud):
  sentra projects           List remote projects
  sentra history            List remote commit history (all projects)
  sentra commits <project>  List commits for a project
  sentra files <project> [--at <commit>]
                           List files for a project (optionally at a commit)
  sentra export <project> [--at <commit>] [--out <dir>]
                           Download and decrypt files into a folder
  sentra sync [--out <dir>] Download latest env files and write them locally
  sentra push               Push pending local commits to remote

Local workflow:
  sentra scan               Scan repos under scan root for env files
  sentra add [path]         Stage env files (default: .)
  sentra status             Show local staged/changed env files
  sentra commit -m <msg>    Create a local commit from staged env files
  sentra log [all|pending|pushed|rm <id>|clear|prune <id|all>|verify]
                           Manage local commit log

Storage (BYOS):
  sentra storage setup      Configure S3-compatible storage
  sentra storage status     Show current storage config
  sentra storage test       Test storage connectivity
  sentra storage reset      Remove storage config

Maintenance:
  sentra doctor             Diagnose connectivity/config issues
  sentra wipe               Delete ALL local Sentra state
`))
}

func runScan() error {
	verbosef("Starting scan operation...")
	scanRoot, err := resolveScanRoot()
	if err != nil {
		return err
	}
	verbosef("Scan root: %s", scanRoot)

	sp := startSpinner(fmt.Sprintf("Scanning %s...", scanRoot))

	projects, err := scanner.Scan(scanRoot)
	if err != nil {
		sp.StopInfo("")
		return err
	}

	envCount := 0
	for _, p := range projects {
		envCount += len(p.EnvFiles)
		if isVerbose() {
			verbosef("Project: %s (%d env file(s))", p.RootPath, len(p.EnvFiles))
		}
	}

	sp.StopSuccess(fmt.Sprintf("✔ %d projects found", len(projects)))
	verbosef("Scan completed: %d project(s), %d env file(s) total", len(projects), envCount)

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
	successf("✔ %d env files detected", envCount)
	fmt.Println()

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
