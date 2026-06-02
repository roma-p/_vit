package clicore

import (
	"context"
	"fmt"

	"vit"
	"vit/internal/types"
)

type CmdTree struct {
	Name        string
	Description string
	SubCommands map[string]*CmdTree
	Run         func(ctx context.Context, client *vit.Client, argParser *CmdParser) (types.Result, error)
	ArgParser   *CmdParser
	NoPyAPI     bool // If true, this command will not be included in the generated Python API
	ResultType  any  // Example instance of the result type (e.g., AddResult{}) for Python API generation
}

func (c *CmdTree) BuildHelp() []string {
	ret := []string{""}
	if len(c.SubCommands) > 0 {
		ret = append(ret, "Available subcommands:")
		for name, cmd := range c.SubCommands {
			ret = append(ret, fmt.Sprintf("  %-15s %s", name, cmd.Description))
		}
	}
	return ret
}

type FindCmdResult struct {
	Code      FindCmdCode
	CmdName   string
	CmdFunc   func(ctx context.Context, client *vit.Client, argParser *CmdParser) (types.Result, error) // if nil: means asking for usage.
	Args      []string
	Usage     []string
	CmdParser *CmdParser
}

type FindCmdCode int

const (
	CmdFound FindCmdCode = iota
	CmdFoundButNoArgs
	CmdFoundAndHelpAsked
	OnCmdBranchButNoArgs
	OnCmdBranchAndHelpAsked
	CmdNotFound
)

func (c *CmdTree) FindCmdTree(args []string) FindCmdResult {
	posArgs, flagArgs := splitPosFlagArgs(args)
	ret := c.find(c.Name, posArgs)

	ret.Args = append(ret.Args, flagArgs...)

	isHelpAsked := false
	for _, f := range flagArgs {
		if f == "-h" || f == "--help" {
			isHelpAsked = true
		}
	}
	if isHelpAsked && ret.Code == OnCmdBranchButNoArgs {
		ret.Code = OnCmdBranchAndHelpAsked
	} else if isHelpAsked && (ret.Code == CmdFoundButNoArgs || ret.Code == CmdFound) {
		ret.Code = CmdFoundAndHelpAsked
	}
	return ret
}

func (c *CmdTree) find(commandName string, args []string) FindCmdResult {
	if len(args) == 0 {
		if c.Run == nil {
			return FindCmdResult{
				Code:    OnCmdBranchButNoArgs,
				CmdName: commandName,
				CmdFunc: nil,
				Args:    args,
				Usage:   c.BuildHelp(),
			}
		} else {
			// Check if the command requires positional arguments
			if c.ArgParser != nil && len(c.ArgParser.PosArgs) == 0 {
				// Command has 0 required positional args - this is valid
				return FindCmdResult{
					Code:      CmdFound,
					CmdName:   commandName,
					CmdFunc:   c.Run,
					Args:      args,
					CmdParser: c.ArgParser,
				}
			} else {
				// Command requires positional args but none provided
				return FindCmdResult{
					Code:      CmdFoundButNoArgs,
					CmdName:   commandName,
					CmdFunc:   nil,
					Args:      args,
					CmdParser: c.ArgParser,
					Usage:     c.BuildHelp(),
				}
			}
		}
	}

	subCmdName := args[0]
	if subCmd, ok := c.SubCommands[subCmdName]; ok {
		return subCmd.find(commandName+" "+subCmdName, args[1:])
	} else if c.Run != nil {
		return FindCmdResult{
			Code:      CmdFound,
			CmdName:   commandName,
			CmdFunc:   c.Run,
			Args:      args,
			CmdParser: c.ArgParser,
		}
	} else {
		return FindCmdResult{
			Code:    CmdNotFound,
			CmdName: subCmdName,
			CmdFunc: nil,
			Args:    nil,
			Usage:   c.BuildHelp(),
		}
	}
}
