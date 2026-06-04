package clicore

import (
	"context"
	"testing"

	"vit"
	"vit/internal/types"

	testutils "vit/internal/testhelpers"
)

func placeHolderFunc(
	ctx context.Context, client *vit.Client, argParser *CmdParser,
) (types.Result, error) {
	return &types.StringListResult{}, nil
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
	testutils.AssertEqual(t, result.Code, CmdFound)
	testutils.AssertEqual(t, result.CmdName, "vit init")
	testutils.AssertSliceEqual(t, result.Args, []string{"some/path"})
}

func TestFindCmdTreeCmdFoundAtNestedLevel(t *testing.T) {
	result := cmdTreeTest.FindCmdTree([]string{"handle", "add", "some/path", "handle", "other/path"})
	testutils.AssertEqual(t, result.Code, CmdFound)
	testutils.AssertEqual(t, result.CmdName, "vit handle add")
	testutils.AssertSliceEqual(t, result.Args, []string{"some/path", "handle", "other/path"})
	testutils.AssertEqual(t, result.CmdParser, handleParser)
}

func TestFindCmdTreeCmdFoundButNoArgs(t *testing.T) {
	// -v and -json shall be ignored, not considered as true args (special args to manage output)
	result := cmdTreeTest.FindCmdTree([]string{"init", "-v", "-json"})
	testutils.AssertEqual(t, result.Code, CmdFoundButNoArgs)
}

func TestFindCmdTreeCmdFoundAndHelpAsked(t *testing.T) {
	result := cmdTreeTest.FindCmdTree([]string{"init", "-h", "-v", "-json"})
	testutils.AssertEqual(t, result.Code, CmdFoundAndHelpAsked)
}

func TestFindCmdTreeCmdBranchButNoArgs(t *testing.T) {
	result := cmdTreeTest.FindCmdTree([]string{"handle", "-v", "-json"})
	testutils.AssertEqual(t, result.Code, OnCmdBranchButNoArgs)
}

func TestFindCmdTreeOnCmdBranchAndHelpAsked(t *testing.T) {
	result := cmdTreeTest.FindCmdTree([]string{"handle", "-h", "-json"})
	testutils.AssertEqual(t, result.Code, OnCmdBranchAndHelpAsked)
}

func TestFindCmdTreeOnCmdNotFound(t *testing.T) {
	result := cmdTreeTest.FindCmdTree([]string{"handle", "invalid", "someOtherArg"})
	testutils.AssertEqual(t, result.Code, CmdNotFound)
}

func TestFindCmdTreeOnNoCmd(t *testing.T) {
	result := cmdTreeTest.FindCmdTree([]string{"-v"})
	testutils.AssertEqual(t, result.Code, OnCmdBranchButNoArgs)
}

func TestFindCmdTreeGeneralHelp(t *testing.T) {
	result := cmdTreeTest.FindCmdTree([]string{"-h"})
	testutils.AssertEqual(t, result.Code, OnCmdBranchAndHelpAsked)
}
