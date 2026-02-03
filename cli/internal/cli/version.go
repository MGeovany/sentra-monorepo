package cli

import (
	"fmt"
	"runtime/debug"
	"strings"
)

// Version information is typically injected at build time via -ldflags.
// Defaults are intentionally unhelpful so we can fall back to build info.
var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

func printVersion() {
	v := strings.TrimSpace(Version)
	if info, ok := debug.ReadBuildInfo(); ok {
		mv := strings.TrimSpace(info.Main.Version)
		if (v == "" || v == "dev") && mv != "" && mv != "(devel)" {
			v = mv
		}
	}
	if v == "" {
		v = "dev"
	}

	out := "sentra " + v
	if c := strings.TrimSpace(Commit); c != "" {
		out += " (" + c + ")"
	}
	if d := strings.TrimSpace(Date); d != "" {
		out += " " + d
	}
	fmt.Println(out)
}
