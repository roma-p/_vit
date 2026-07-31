package cli

import (
	"context"

	"vit"
	"vit/internal/cli/clicore"
	"vit/internal/types"
)

func buildAddArgParser() *clicore.CmdParser {
	return clicore.NewArgParser("add", []string{"assetPath"}, []string{"src"})
}

func runAdd(ctx context.Context, client *vit.Client, argParser *clicore.CmdParser) (types.Result, error) {
	return client.Add(
		ctx,
		argParser.Arg("assetPath"),
		argParser.Arg("src"),
	)
}
