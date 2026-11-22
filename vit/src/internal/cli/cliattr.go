package cli

import (
	"context"
	"fmt"
	"strings"

	"vit"
	"vit/internal/cli/clicore"
	"vit/internal/types"
)

func buildAttrSetArgParser() *clicore.CmdParser {
	ret := clicore.NewArgParser("attr set", []string{"path"}, []string{})
	ret.AddVariadic("values", "attributes to set, as key=value")
	return ret
}

func runAttrSet(
	ctx context.Context,
	client *vit.Client,
	argParser *clicore.CmdParser,
) (types.Result, error) {
	values, err := parseAttrValues(argParser.GetVariadic("values"))
	if err != nil {
		return nil, clicore.NewUsageError(argParser.Name, []string{}, clicore.ErrInvalidCommand, err)
	}
	return client.AttrSet(ctx, argParser.GetArg("path"), values)
}

func buildAttrRmArgParser() *clicore.CmdParser {
	ret := clicore.NewArgParser("attr rm", []string{"path"}, []string{})
	ret.AddVariadic("keys", "keys of the attributes to remove")
	return ret
}

func runAttrRm(
	ctx context.Context,
	client *vit.Client,
	argParser *clicore.CmdParser,
) (types.Result, error) {
	return client.AttrRm(ctx, argParser.GetArg("path"), argParser.GetVariadic("keys"))
}

func buildAttrLsArgParser() *clicore.CmdParser {
	return clicore.NewArgParser("attr ls", []string{"path"}, []string{})
}

func runAttrLs(
	ctx context.Context,
	client *vit.Client,
	argParser *clicore.CmdParser,
) (types.Result, error) {
	return client.AttrLs(ctx, argParser.GetArg("path"))
}

// -- FILE LOCAL -------------------------------------------------------------

func parseAttrValues(args []string) (map[string]string, error) {
	values := make(map[string]string, len(args))
	for _, kv := range args {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return nil, fmt.Errorf("expected key=value, got %q", kv)
		}
		if _, dup := values[k]; dup {
			return nil, fmt.Errorf("attribute %q given twice", k)
		}
		values[k] = v
	}
	return values, nil
}
