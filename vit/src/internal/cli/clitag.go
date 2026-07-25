package cli

import (
	"context"

	"vit"
	"vit/internal/cli/clicore"
	"vit/internal/types"
)

func buildTagArgParser() *clicore.CmdParser {
	ret := clicore.NewArgParser("tag", []string{"path", "family", "message"}, []string{})
	return ret
}

func runTag(
	ctx context.Context,
	client *vit.Client,
	argParser *clicore.CmdParser,
) (types.Result, error) {
	return client.Tag(ctx, argParser.Arg("path"), argParser.Arg("family"), argParser.Arg("message"))
}
