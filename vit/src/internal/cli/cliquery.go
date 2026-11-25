package cli

import (
	"context"

	"vit"
	"vit/internal/cli/clicore"
	"vit/internal/types"
)

func buildQueryArgParser() *clicore.CmdParser {
	ret := clicore.NewArgParser("query", []string{}, []string{"path"})
	_ = ret.AddArgCommaString("with", "keys the assets must have")
	_ = ret.AddArgCommaString("without", "keys the assets must not have")
	_ = ret.AddArgCommaString("eq", "key=value pairs the assets must have")
	_ = ret.AddArgCommaString("fetch", "keys whose values to return")
	_ = ret.AddFlag("asset", false, "return the asset paths")
	_ = ret.AddFlag("distinct", false, "return each tuple of fetched values once")
	_ = ret.AddFlag("set", false, "return the values found for each fetched key")
	return ret
}

func runQuery(
	ctx context.Context,
	client *vit.Client,
	argParser *clicore.CmdParser,
) (types.Result, error) {
	req, err := queryRequestFromArgs(argParser)
	if err != nil {
		return nil, clicore.NewUsageError(argParser.Name, []string{}, clicore.ErrInvalidCommand, err)
	}
	path := argParser.GetArg("path")
	if path == "" {
		path = "."
	}
	argParser.UsePager = true
	return client.Query(ctx, path, req)
}

// -- FILE LOCAL -------------------------------------------------------------

func queryRequestFromArgs(argParser *clicore.CmdParser) (types.QueryRequest, error) {
	eq, err := parseAttrValues(argParser.GetCommaString("eq"))
	if err != nil {
		return types.QueryRequest{}, err
	}
	return types.QueryRequest{
		With:     argParser.GetCommaString("with"),
		Without:  argParser.GetCommaString("without"),
		Eq:       eq,
		Fetch:    argParser.GetCommaString("fetch"),
		Asset:    *argParser.GetFlag("asset").(*bool),
		Distinct: *argParser.GetFlag("distinct").(*bool),
		Set:      *argParser.GetFlag("set").(*bool),
	}, nil
}
