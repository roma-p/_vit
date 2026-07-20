package cli

import (
	"context"

	"vit"
	"vit/internal/cli/clicore"
	"vit/internal/types"
)

func runInit(ctx context.Context, client *vit.Client, argParser *clicore.CmdParser) (types.Result, error) {
	return client.InitRepo(ctx, argParser.Arg("path"))
}
