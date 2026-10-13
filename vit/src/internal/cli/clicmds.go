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
		"log": {
			Name:        "Log",
			Description: "Show the project-wide activity log (newest first)",
			Run:         runLog,
			ArgParser:   buildLogArgParser(),
			ResultType:  types.HistoryLogResult{},
		},
		"ls": {
			Name:        "Ls",
			Description: "List a directory's children, an asset's branches/tags, or an object's details + sources",
			Run:         runLs,
			ArgParser:   buildLsArgParser(),
			ResultType:  types.LsResult{},
		},
		"snapshot": {
			Name:        "Snapshot",
			Description: "Snapshot a given branch if there is change to snapshot",
			Run:         runSnapshot,
			ArgParser:   buildSnapshotArgParser(),
			ResultType:  types.StringResult{},
		},
		"branch": {
			Name:        "Branch",
			Description: "Manage branches",
			SubCommands: map[string]*clicore.CmdTree{
				"add": {
					Name:        "Add branch",
					Description: "Create a new branch from a branch/commit/tag/snapshot (source branch must be clean)",
					Run:         runBranchAdd,
					ArgParser:   buildBranchAddArgParser(),
					ResultType:  types.StringResult{},
				},
			},
		},
		"tag": {
			Name:        "Tag",
			Description: "Tag a given ref",
			Run:         runTag,
			ArgParser:   buildTagArgParser(),
			ResultType:  types.SourcePullResult{},
		},
		"source": {
			Name:        "Souce",
			Description: "Manage sources",
			SubCommands: map[string]*clicore.CmdTree{
				"add": {
					Name:        "Add source",
					Description: "Add a new source to asset",
					Run:         runSourceAdd,
					ArgParser:   buildSourceAddArgParser(),
					ResultType:  types.StringResult{},
				},
				"pull": {
					Name:        "Pull sources",
					Description: "Update an asset's sources to their latest pinned version",
					Run:         runSourcePull,
					ArgParser:   buildSourcePullArgParser(),
					ResultType:  types.StringListResult{},
				},
				"push": {
					Name:        "Push source",
					Description: "Re-pin every live consumer of a floating tag family or branch",
					Run:         runSourcePush,
					ArgParser:   buildSourcePushArgParser(),
					ResultType:  types.SourcePushResult{},
				},
			},
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
