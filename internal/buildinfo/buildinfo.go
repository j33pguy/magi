package buildinfo

import "runtime/debug"

const Version = "0.5.0"

type Info struct {
	Version  string `json:"version"`
	Revision string `json:"revision,omitempty"`
	Modified bool   `json:"modified,omitempty"`
}

func Current() Info {
	info := Info{Version: Version}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return info
	}
	for _, setting := range bi.Settings {
		switch setting.Key {
		case "vcs.revision":
			info.Revision = setting.Value
		case "vcs.modified":
			info.Modified = setting.Value == "true"
		}
	}
	return info
}
