package cli

import (
	"vit/internal/cli/clicore"
	"vit/internal/types"
)

func init() {
	// Set rootCmd after initialization to avoid circular dependency with genpy command!
	rootCmd = &cmdDefinition
}

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
		"add": {
			Name:        "Add",
			Description: "Add a new asset to repository",
			Run:         runAdd,
			ArgParser:   buildAddArgParser(),
			ResultType:  types.StringResult{},
		},
		"status": {
			Name:        "Status",
			Description: "List all non committed change for a given branch",
			Run:         runStatus,
			ArgParser:   buildStatusArgParser(),
			ResultType:  types.StatusResult{},
		},
		"commit": {
			Name:        "Commit",
			Description: "Commit a given branch if there is change to commit",
			Run:         runCommit,
			ArgParser:   buildCommitArgParser(),
			ResultType:  types.StringResult{},
		},
		"tag": {
			Name:        "Tag",
			Description: "Tag a given ref",
			Run:         runTag,
			ArgParser:   buildTagArgParser(),
			ResultType:  types.StringResult{},
		},
		"dev": {
			Name:        "Dev",
			Description: "Developer and internal commands",
			SubCommands: map[string]*clicore.CmdTree{
				"genpy": {
					Name:        "GenPy",
					Description: "Generate Python API client",
					Run:         runGenPy,
					ArgParser:   clicore.NewArgParser("genpy", []string{"output"}, []string{}),
					NoPyAPI:     true, // Don't include this command in generated API
				},
			},
		},
	},
}
