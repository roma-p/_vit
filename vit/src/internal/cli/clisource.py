package cli

import (
	"context"

	"vit"
)

func buildSourceAddArgParser() *CmdParser {
	return NewArgParser("add", []string{"dstPath", "srcPath", "srcName"}, []string{})
}

func runSourceAdd(ctx context.Context, client *vit.Client, argParser *CmdParser) (CliResult, error) {
	result, err := client.SourceAdd(
		ctx,
		argParser.GetArg("dstPath"),
		argParser.GetArg("srcPath"),
		argParser.GetArg("srcName"),
	)

	if err != nil {
		return nil, err
	} else {
		return &AddSourceResult{
			Source:            result.SourceName,
			PathNewSource:     result.PathNewSource,
			PathWithNewSource: result.PathWithNewSource,
		}, nil
	}
}
