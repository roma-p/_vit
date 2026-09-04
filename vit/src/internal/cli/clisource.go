package cli

import (
	"context"

	"vit"
	"vit/internal/cli/clicore"
	"vit/internal/types"
)

func buildSourceAddArgParser() *clicore.CmdParser {
	return clicore.NewArgParser("source add", []string{"dstPath", "srcPath", "srcName"}, []string{})
}

func runSourceAdd(
	ctx context.Context,
	client *vit.Client,
	argParser *clicore.CmdParser,
) (types.Result, error) {
	return client.SourceAdd(
		ctx,
		argParser.Arg("dstPath"),
		argParser.Arg("srcPath"),
		argParser.Arg("srcName"),
	)
}
