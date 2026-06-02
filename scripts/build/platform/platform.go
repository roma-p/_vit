package platform

import "fmt"

type Platform struct {
	OS   string
	Arch string
}

func (p *Platform) String() string {
	return fmt.Sprintf("%s-%s", p.OS, p.Arch)
}

var All = []Platform{
	{"darwin", "arm64"},
	{"darwin", "amd64"},
	{"linux", "amd64"},
	{"linux", "arm64"},
	{"windows", "amd64"},
}

type Forge interface {
	Name() string
	Validate() error
	CreateRelease(version, changelog string) (string, error)
	UploadAsset(uploadURL, filepath string) error
}

type ReleaseConfig struct {
	Version   string
	Commit    string // Optional: git commit hash
	Date      string
	Changelog string
	DistDir   string
	Checksums map[string]string // filename -> sha256
}
