package cli

import (
	"context"

	"vit"
	"vit/internal/cli/clicore"
	"vit/internal/types"
)

func buildBranchAddArgParser() *clicore.CmdParser {
	ret := clicore.NewArgParser("branch add", []string{"path"}, []string{})
	ret.AddVariadic("names", "names of the branches to create")
	return ret
}

func runBranchAdd(
	ctx context.Context,
	client *vit.Client,
	argParser *clicore.CmdParser,
) (types.Result, error) {
	return client.Branch(ctx, argParser.GetArg("path"), argParser.GetVariadic("names"))
}
