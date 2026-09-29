package cli

import (
	"context"

	"vit"
	"vit/internal/cli/clicore"
	"vit/internal/types"
)

func buildStatusArgParser() *clicore.CmdParser {
	ret := clicore.NewArgParser("status", []string{"branchPath"}, []string{})
	return ret
}

func runStatus(ctx context.Context, client *vit.Client, argParser *clicore.CmdParser) (types.Result, error) {
	return client.Status(ctx, argParser.GetArg("branchPath"))
}
