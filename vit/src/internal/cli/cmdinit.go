package cli

import (
	"context"

	"vit"
	"vit/internal/types"
	"vit/internal/cli/clicore"
)

func runInit(ctx context.Context, client *vit.Client, argParser *clicore.CmdParser) (types.Result, error) {
	// path, err := client.InitRepo(ctx, argParser.GetArg("path"))
	// if err != nil {
	// 	return nil, err
	// }
	return &types.StringResult{String: "prout"}, nil
}
