package cli

import (
	"context"

	"vit"
	"vit/internal/cli/clicore"
	"vit/internal/types"
)

func buildSnapshotArgParser() *clicore.CmdParser {
	ret := clicore.NewArgParser("snapshot", []string{"branchPath", "message"}, []string{})
	return ret
}

func runSnapshot(
	ctx context.Context,
	client *vit.Client,
	argParser *clicore.CmdParser,
) (types.Result, error) {
	return client.Snapshot(ctx, argParser.GetArg("branchPath"), argParser.GetArg("message"))
}
