package cli

import (
	"context"

	"vit"
	"vit/internal/cli/clicore"
	"vit/internal/types"
)

func runGenRust(ctx context.Context, client *vit.Client, argParser *clicore.CmdParser) (types.Result, error) {
	if err := rootCmd.GenRustAPI(argParser.GetArg("output")); err != nil {
		return nil, err
	}
	return &types.EmptyResult{}, nil
}
