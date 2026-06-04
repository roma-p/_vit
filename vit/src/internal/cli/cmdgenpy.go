package cli

import (
	"context"

	"vit"
	"vit/internal/cli/clicore"
	"vit/internal/types"
)

// // rootCmd will be set after cmdDefinition is initialized to avoid circular dependency
var rootCmd *clicore.CmdTree

func runGenPy(ctx context.Context, client *vit.Client, argParser *clicore.CmdParser) (types.Result, error) {
	outputPath := argParser.GetArg("output")

	err := rootCmd.GenPythonAPI(outputPath)
	if err != nil {
		return nil, types.NewTopLevelError("genpy", err)
	}
	return &types.EmptyResult{}, nil
}
