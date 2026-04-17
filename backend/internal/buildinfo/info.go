package buildinfo

import (
	"runtime/debug"
	"strings"
	"sync"
)

// AppVersion is injected at build time via -ldflags.
var AppVersion string

var (
	versionOnce sync.Once
	version     string
)

func Version() string {
	versionOnce.Do(func() {
		v := strings.TrimSpace(AppVersion)
		if v == "" {
			if info, ok := debug.ReadBuildInfo(); ok {
				built := strings.TrimSpace(info.Main.Version)
				if built != "" && built != "(devel)" {
					v = built
				}
			}
		}
		version = v
	})
	return version
}
