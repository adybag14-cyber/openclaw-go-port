package buildinfo

var (
	version = "dev"
	commit  = "none"
	builtAt = "unknown"
)

type Info struct {
	Service string `json:"service"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
	BuiltAt string `json:"built_at"`
}

func Default() Info {
	return Info{
		Service: "openclaw-go",
		Version: version,
		Commit:  commit,
		BuiltAt: builtAt,
	}
}
