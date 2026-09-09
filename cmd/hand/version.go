package main

import (
	"fmt"
	"runtime/debug"
)

// Release builds inject these values. Ordinary go build/go install retain
// module/VCS metadata without requiring a Makefile.
var version = "dev"
var buildCommit = ""

func versionString() string {
	v, commit := version, buildCommit
	if info, ok := debug.ReadBuildInfo(); ok {
		if v == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
			v = info.Main.Version
		}
		if commit == "" {
			dirty := false
			for _, setting := range info.Settings {
				switch setting.Key {
				case "vcs.revision":
					commit = setting.Value
				case "vcs.modified":
					dirty = setting.Value == "true"
				}
			}
			if dirty && commit != "" {
				commit += "-dirty"
			}
		}
	}
	if commit == "" {
		commit = "unknown"
	}
	return fmt.Sprintf("%s (commit %s)", v, commit)
}
