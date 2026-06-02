package clicore

import (
	"context"
	"testing"

	"vit"

	testutils "vit/internal/testhelpers"
)

func placeHolderFunc(
	ctx context.Context, client *vit.Client, argParser *CmdParser,
) (CliResult, error) {
	return &StringListResult{}, nil
}

var initParser = NewArgParser("init", []string{"path"}, []string{})

var handleParser = NewArgParser("handle add", []string{"assetPath", "handleName", "repoPath"}, []string{})

var cmdTreeTest = CmdTree{
	Name:        "vit",
	Description: "a versionning control software for vfx",
	SubCommands: map[string]*CmdTree{
		"init": {
			Name:        "Init",
			Description: "Initialize a repository",
			Run:         placeHolderFunc,
			ArgParser:   initParser,
		},
		"handle": {
			Name:        "Handle",
			Description: "Manage handles",
			SubCommands: map[string]*CmdTree{
				"add": {
					Name:        "Add handle",
					Description: "Add a new handle to asset",
					Run:         placeHolderFunc,
					ArgParser:   handleParser,
				},
			},
		},
	},
}

func TestFindCmdTreeCmdFoundAtRootLevel(t *testing.T) {
	result := cmdTreeTest.FindCmdTree([]string{"init", "some/path"})
	testutils.AssertEqual(t, result.code, cmdFound)
	testutils.AssertEqual(t, result.cmdName, "vit init")
	testutils.AssertSliceEqual(t, result.args, []string{"some/path"})
}

func TestFindCmdTreeCmdFoundAtNestedLevel(t *testing.T) {
	result := cmdTreeTest.FindCmdTree([]string{"handle", "add", "some/path", "handle", "other/path"})
	testutils.AssertEqual(t, result.code, cmdFound)
	testutils.AssertEqual(t, result.cmdName, "vit handle add")
	testutils.AssertSliceEqual(t, result.args, []string{"some/path", "handle", "other/path"})
	testutils.AssertEqual(t, result.cmdParser, handleParser)
}

func TestFindCmdTreeCmdFoundButNoArgs(t *testing.T) {
	// -v and -json shall be ignored, not considered as true args (special args to manage output)
	result := cmdTreeTest.FindCmdTree([]string{"init", "-v", "-json"})
	testutils.AssertEqual(t, result.code, cmdFoundButNoArgs)
}

func TestFindCmdTreeCmdFoundAndHelpAsked(t *testing.T) {
	result := cmdTreeTest.FindCmdTree([]string{"init", "-h", "-v", "-json"})
	testutils.AssertEqual(t, result.code, cmdFoundAndHelpAsked)
}

func TestFindCmdTreeCmdBranchButNoArgs(t *testing.T) {
	result := cmdTreeTest.FindCmdTree([]string{"handle", "-v", "-json"})
	testutils.AssertEqual(t, result.code, onCmdBranchButNoArgs)
}

func TestFindCmdTreeOnCmdBranchAndHelpAsked(t *testing.T) {
	result := cmdTreeTest.FindCmdTree([]string{"handle", "-h", "-json"})
	testutils.AssertEqual(t, result.code, onCmdBranchAndHelpAsked)
}

func TestFindCmdTreeOnCmdNotFound(t *testing.T) {
	result := cmdTreeTest.FindCmdTree([]string{"handle", "invalid", "someOtherArg"})
	testutils.AssertEqual(t, result.code, cmdNotFound)
}

func TestFindCmdTreeOnNoCmd(t *testing.T) {
	result := cmdTreeTest.FindCmdTree([]string{"-v"})
	testutils.AssertEqual(t, result.code, onCmdBranchButNoArgs)
}

func TestFindCmdTreeGeneralHelp(t *testing.T) {
	result := cmdTreeTest.FindCmdTree([]string{"-h"})
	testutils.AssertEqual(t, result.code, onCmdBranchAndHelpAsked)
}
