package clicore

import (
	"slices"
	"strings"
)

// apiCommand is a runnable command of the tree, as seen by an API generator.
type apiCommand struct {
	Name     string   // e.g. : "init", "branch_add"
	CmdParts []string // e.g. : ["branch", "add"]
	Tree     *CmdTree
}

// walkAPICommands returns the commands to expose, in a stable order so a
// regenerated API only differs when the commands do.
func (c *CmdTree) walkAPICommands(name string, cmdParts []string) []apiCommand {
	if c.NoAPI {
		return nil
	}

	var ret []apiCommand
	if c.Run != nil && c.ArgParser != nil {
		ret = append(ret, apiCommand{Name: name, CmdParts: cmdParts, Tree: c})
	}

	subNames := make([]string, 0, len(c.SubCommands))
	for subName := range c.SubCommands {
		subNames = append(subNames, subName)
	}
	slices.Sort(subNames)

	for _, subName := range subNames {
		// Hyphens are not valid in method names.
		subAPIName := strings.ReplaceAll(subName, "-", "_")
		if name != "" {
			subAPIName = name + "_" + subAPIName
		}
		subParts := append(slices.Clone(cmdParts), subName)
		ret = append(ret, c.SubCommands[subName].walkAPICommands(subAPIName, subParts)...)
	}
	return ret
}

// apiFlagNames returns the command's own flags (not the global ones), sorted.
func (a *CmdParser) apiFlagNames() []string {
	ret := make([]string, 0, len(a.flagDefs))
	for name := range a.flagDefs {
		switch name {
		case "json", "debug", "v", "h":
		default:
			ret = append(ret, name)
		}
	}
	slices.Sort(ret)
	return ret
}
