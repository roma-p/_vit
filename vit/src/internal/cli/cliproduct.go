package cli

import (
	"context"

	"vit"
	"vit/internal/cli/clicore"
	"vit/internal/types"
)

func buildProductAddArgParser() *clicore.CmdParser {
	ret := clicore.NewArgParser("product add", []string{"origin", "target", "name"}, []string{})
	_ = ret.AddFlag("create", false, "create the target asset if it does not exist")
	return ret
}

func runProductAdd(
	ctx context.Context,
	client *vit.Client,
	argParser *clicore.CmdParser,
) (types.Result, error) {
	create := *argParser.GetFlag("create").(*bool)
	return client.ProductAdd(
		ctx,
		argParser.GetArg("origin"),
		argParser.GetArg("target"),
		argParser.GetArg("name"),
		create,
	)
}
