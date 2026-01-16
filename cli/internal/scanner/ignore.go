package scanner

import (
	"bufio"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

type gitIgnoreFile struct {
	dir      string
	patterns []gitIgnorePattern
}

type gitIgnorePattern struct {
	negate            bool
	dirOnly           bool
	anchored          bool
	matchBasenameOnly bool
	regex             *regexp.Regexp
}

func loadGitIgnoreFile(dir string) (gitIgnoreFile, bool, error) {
	filePath := filepath.Join(dir, ".gitignore")

	f, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return gitIgnoreFile{}, false, nil
		}
		return gitIgnoreFile{}, false, err
	}
	defer f.Close()

	var patterns []gitIgnorePattern
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, `\#`) || strings.HasPrefix(line, `\!`) {
			line = line[1:]
		}

		pattern, ok, err := parseGitIgnorePattern(line)
		if err != nil {
			return gitIgnoreFile{}, false, err
		}
		if ok {
			patterns = append(patterns, pattern)
		}
	}
	if err := scanner.Err(); err != nil {
		return gitIgnoreFile{}, false, err
	}

	return gitIgnoreFile{dir: dir, patterns: patterns}, true, nil
}

func parseGitIgnorePattern(line string) (gitIgnorePattern, bool, error) {
	p := gitIgnorePattern{}

	if strings.HasPrefix(line, "!") {
		p.negate = true
		line = strings.TrimPrefix(line, "!")
	}

	if strings.HasSuffix(line, "/") {
		p.dirOnly = true
		line = strings.TrimSuffix(line, "/")
	}

	if strings.HasPrefix(line, "/") {
		p.anchored = true
		line = strings.TrimPrefix(line, "/")
	}

	if line == "" {
		return gitIgnorePattern{}, false, nil
	}

	hasSlash := strings.Contains(line, "/")
	p.matchBasenameOnly = !hasSlash && !p.anchored

	globRegex, err := globToRegex(line)
	if err != nil {
		return gitIgnorePattern{}, false, err
	}

	var fullRegex string
	switch {
	case p.matchBasenameOnly:
		fullRegex = "^" + globRegex + "$"
	case p.anchored:
		fullRegex = "^" + globRegex + "$"
	default:
		// unanchored patterns containing "/" match at any depth
		fullRegex = "(^|.*/)" + globRegex + "$"
	}

	re, err := regexp.Compile(fullRegex)
	if err != nil {
		return gitIgnorePattern{}, false, err
	}
	p.regex = re

	return p, true, nil
}

func globToRegex(glob string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(glob); i++ {
		c := glob[i]

		switch c {
		case '*':
			// handle "**" (match across directories)
			if i+1 < len(glob) && glob[i+1] == '*' {
				b.WriteString(".*")
				i++
				continue
			}
			b.WriteString("[^/]*")
		case '?':
			b.WriteString("[^/]")
		case '.', '+', '(', ')', '|', '^', '$', '{', '}', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case '[':
			// copy character class verbatim until closing bracket
			j := i + 1
			for j < len(glob) && glob[j] != ']' {
				j++
			}
			if j >= len(glob) {
				b.WriteString("\\[")
				continue
			}
			b.WriteString(glob[i : j+1])
			i = j
		default:
			b.WriteByte(c)
		}
	}
	return b.String(), nil
}

func (p gitIgnorePattern) matches(relPathFromIgnoreDir string, isDir bool) bool {
	if p.dirOnly && !isDir {
		return false
	}

	candidate := relPathFromIgnoreDir
	if p.matchBasenameOnly {
		candidate = path.Base(candidate)
	}

	return p.regex.MatchString(candidate)
}
