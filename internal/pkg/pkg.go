package pkg

type PackageInfo struct {
	Filename      string
	Name          string
	Version       string
	Base          string
	Desc          string
	URL           string
	Arch          string
	Packager      string
	License       []string
	Groups        []string
	Depends       []string
	OptDepends    []string
	Provides      []string
	Conflicts     []string
	Replaces      []string
	InstalledSize int64
	Size          int64
	BuildDate     int64
	MD5Sum        string
	SHA256Sum     string
	PGPSig        string
	Backup        []string
	Files         []string
}
