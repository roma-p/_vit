package cli

import (
	"context"

	"vit"
	"vit/internal/cli/clicore"
	"vit/internal/types"
)

func buildLogArgParser() *clicore.CmdParser {
	return clicore.NewArgParser("log", []string{}, []string{})
}

func runLog(
	ctx context.Context,
	client *vit.Client,
	argParser *clicore.CmdParser,
) (types.Result, error) {
	argParser.UsePager = true
	// "." → the cwd, used only to locate the enclosing repo.
	return client.Log(ctx, ".")
}
