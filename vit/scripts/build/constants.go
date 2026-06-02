package main

import (
	//"vit-scripts/build/forge"
	"vit-scripts/build/platform"
)

type BinBuild struct {
	Platform platform.Platform
	Binary   string // Path to binary
}

const (
	RepoURL      = "https://github.com/roma-p/vit"
	RepoURLFzf   = "https://github.com/junegunn/fzf/"
	fzfVersion   = "0.67.0"
)

const (
	buildPathPortable = "bin/vit-portable"
	buildPathMinimal  = "bin/vit-minimal"
)

// VitForge :: Current Forge for vit is Github (maybe codeberg soon?)
// var VitForge = &forge.Github{
// 	Repo: RepoURL,
// 	API:  "https://api.github.com/repos/roma-p/vit",
// }
