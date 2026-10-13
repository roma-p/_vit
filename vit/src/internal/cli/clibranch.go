package cli

import (
	"context"

	"vit"
	"vit/internal/cli/clicore"
	"vit/internal/types"
)

func buildBranchAddArgParser() *clicore.CmdParser {
	return clicore.NewArgParser("branch add", []string{"path", "name"}, []string{})
}

func runBranchAdd(
	ctx context.Context,
	client *vit.Client,
	argParser *clicore.CmdParser,
) (types.Result, error) {
	return client.Branch(ctx, argParser.GetArg("path"), argParser.GetArg("name"))
}
