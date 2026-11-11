package rekha

import "runtime/debug"

// Version is the module version this binary was built from, as Go
// records it: a tag such as v1.2.0 when installed with go install, or
// "dev" from a source checkout or when the build carries no version.
func Version() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		switch v := info.Main.Version; v {
		case "", "(devel)":
		default:
			return v
		}
	}
	return "dev"
}
