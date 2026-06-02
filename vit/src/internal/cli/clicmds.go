package cli

import (
	"vit/internal/cli/clicore"
	"vit/internal/types"
)

// WHUT?
// func init() {
// 	// Set rootCmd after initialization to avoid circular dependency
// 	rootCmd = &cmdDefinition
// }

var cmdDefinition = clicore.CmdTree{
	Name:        "vit",
	Description: "a versionning control software for vfx",
	SubCommands: map[string]*clicore.CmdTree{
		"init": {
			Name:        "Init",
			Description: "Initialize a repository",
			Run:         runInit,
			ArgParser:   clicore.NewArgParser("init", []string{"path"}, []string{}),
			ResultType:  types.StringResult{},
		},
	},
}
