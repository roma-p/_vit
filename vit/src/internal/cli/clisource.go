package cli

import (
	"context"
	"fmt"

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
		argParser.GetArg("dstPath"),
		argParser.GetArg("srcPath"),
		argParser.GetArg("srcName"),
	)
}

func buildSourcePullArgParser() *clicore.CmdParser {
	ret := clicore.NewArgParser("source pull", []string{"path"}, []string{})
	ret.AddVariadic("sources", "all source names to pull")
	ret.AddFlag("all", false, "pull all sources")
	return ret
}

func runSourcePull(
	ctx context.Context,
	client *vit.Client,
	argParser *clicore.CmdParser,
) (types.Result, error) {

	sourceList := argParser.Variadic
	all := *argParser.GetFlag("all").(*bool)

	var sources []string
	if all && len(sourceList) > 0 {
		return nil, clicore.NewUsageError(
			argParser.Name,
			[]string{},
			clicore.ErrInvalidCommand,
			fmt.Errorf("either pull all source using -all or pull some source by listing them"),
		)
	} else if all {
		sources = []string{}
	} else {
		sources = sourceList
	}

	return client.SourcePull(ctx, argParser.GetArg("path"), sources)
}
