package cli

import (
	"context"

	"vit"
	"vit/internal/cli/clicore"
	"vit/internal/types"
)

func buildLsArgParser() *clicore.CmdParser {
	ret := clicore.NewArgParser("ls", []string{}, []string{"path"})
	_ = ret.AddFlag("f", false, "fuzzy-pick the target with fzf")
	return ret
}

func runLs(
	ctx context.Context,
	client *vit.Client,
	argParser *clicore.CmdParser,
) (types.Result, error) {
	path := argParser.GetArg("path")
	if fuzzy, ok := argParser.GetFlag("f").(*bool); ok && *fuzzy {
		sel, err := fuzzyFind(ctx, client)
		if err != nil {
			return nil, err
		}
		path = sel
	}
	if path == "" {
		path = "."
	}
	return client.Ls(ctx, path)
}
