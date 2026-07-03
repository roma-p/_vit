package cli

import (
	"context"

	"vit"
	"vit/internal/cli/clicore"
	"vit/internal/types"
)

func buildAddArgParser() *clicore.CmdParser {
	ret := clicore.NewArgParser("add", []string{"assetPath", "src"}, []string{})
	ret.Bool("d", false, "Will create a directory instead of a file. Not needed if src is a directory")
	return ret
}

func runAdd(ctx context.Context, client *vit.Client, argParser *clicore.CmdParser) (types.Result, error) {
	return client.Add(
		ctx,
		argParser.GetArg("assetPath"),
		argParser.GetArg("src"),
		// *argParser.GetFlag("d").(*bool),
	)
}
