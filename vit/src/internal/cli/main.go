package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"vit"
	"vit/internal/cli/clicore"
)

// Main runs the CLI and returns an exit code
func Main() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	findRet := cmdDefinition.FindCmdTree(os.Args[1:])

	// handling command errors / edge case.
	switch findRet.Code {
	case clicore.CmdFoundButNoArgs:
		return handleCmdFoundButNoArgs(&findRet)
	case clicore.CmdFoundAndHelpAsked:
		return handleCmdFoundAndHelpAsked(&findRet)
	case clicore.OnCmdBranchButNoArgs:
		return handleOnBranchButNoArgs(&findRet)
	case clicore.OnCmdBranchAndHelpAsked:
		return handleOnBranchAndHelpAsked(&findRet)
	case clicore.CmdNotFound:
		return handleCmdNotFound(&findRet)
	case clicore.CmdFound:
		{
		} // normal scenario handled below.
	default:
		panic("unreachable")
	}

	subcmdparser := findRet.CmdParser
	usageErr := subcmdparser.Parse(findRet.Args)

	output := clicore.NewOutput(clicore.OutputOpt{
		JSON:    *subcmdparser.GetFlag("json").(*bool),
		Debug:   *subcmdparser.GetFlag("debug").(*bool),
		Verbose: *subcmdparser.GetFlag("v").(*bool),
	})

	if usageErr != nil {
		return handleCmdError(output, usageErr)
	}

	client, err := vit.NewClient(ctx, output.Logger, output)
	if err != nil {
		return output.Process(nil, err)
	}

	cliResult, err := findRet.CmdFunc(ctx, client, subcmdparser)
	client.Dispose()
	return output.Process(cliResult, err)
}

func handleCmdFoundButNoArgs(findRet *clicore.FindCmdResult) int {
	output := buildMinimalOutput()
	output.HumanReadableToStd(findRet.CmdParser.Usage, true)
	return clicore.ExitUsageError
}

func handleCmdFoundAndHelpAsked(findRet *clicore.FindCmdResult) int {
	output := buildMinimalOutput()
	output.HumanReadableToStd(findRet.CmdParser.Usage, false)
	return clicore.ExitSuccess
}

func handleOnBranchButNoArgs(findRet *clicore.FindCmdResult) int {
	output := buildMinimalOutput()
	output.HumanReadableToStd(findRet.Usage, true)
	return clicore.ExitUsageError
}

func handleOnBranchAndHelpAsked(findRet *clicore.FindCmdResult) int {
	output := buildMinimalOutput()
	output.HumanReadableToStd(findRet.Usage, false)
	return clicore.ExitSuccess
}

func handleCmdNotFound(findRet *clicore.FindCmdResult) int {
	output := buildMinimalOutput()
	stderr := []string{fmt.Sprintf("unknown command: %s", findRet.CmdName)}
	stderr = append(stderr, findRet.Usage...)
	output.HumanReadableToStd(stderr, true)
	output.Logger.Error("invalid command", "command", findRet.CmdName)
	return clicore.ExitUsageError
}

func handleCmdError(output *clicore.Output, err *clicore.UsageError) int {
	output.HumanReadableToStd([]string{err.Message}, true)
	output.Logger.Error(
		"invalid command",
		"command", err.Command,
		"errorType", err.ErrorType,
	)
	return clicore.ExitUsageError
}

func buildMinimalOutput() *clicore.Output {
	// minimal output use to capture basic flagset
	// to try to catch stderr/out/log related flags in case of error.
	tmpFS := flag.NewFlagSet("tmp", flag.ContinueOnError)
	tmpFS.SetOutput(io.Discard)

	json := tmpFS.Bool("json", false, "")
	debug := tmpFS.Bool("debug", false, "")
	verbose := tmpFS.Bool("v", false, "")

	return clicore.NewOutput(clicore.OutputOpt{
		JSON:    *json,
		Debug:   *debug,
		Verbose: *verbose,
	})
}
