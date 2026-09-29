package cli

import (
	"context"

	"vit"
	"vit/internal/cli/clicore"
	"vit/internal/types"
)

func buildCommitArgParser() *clicore.CmdParser {
	ret := clicore.NewArgParser("commit", []string{"branchPath", "message"}, []string{})
	return ret
}

func runCommit(
	ctx context.Context,
	client *vit.Client,
	argParser *clicore.CmdParser,
) (types.Result, error) {
	return client.Commit(ctx, argParser.GetArg("branchPath"), argParser.GetArg("message"))
}
