package clicore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"time"

	"vit"
	"vit/internal/types"
)

// Output handles all CLI output formatting and routing.
//
// Output is the central component for presenting results to users,
// supporting two distinct modes:
//
//   - Human-readable mode: Formatted text output for terminal users
//   - JSON mode: NDJSON for programmatic consumption
//
// Four types of data are produced by Output
// (available in both human-readable mode and JSON mode):
//
//   - CLI Result: when the command went well and produced the expected result.
//     See result_types.go for all possible types.
//   - Errors: see types/errors.go for all possible types.
//   - Progress: used when a long operation is taking place.
//     In HR mode, a progress bar.
//     In JSON mode, see types/progress.go.
//   - Logs.
//
// In HR mode, the redirection works as expected: stdout only has results,
// everything else on stderr to not pollute stdout.
// In JSON mode, the redirection is different:
// logger is on stderr, everything else is on stdout.
//
// To sum it up:
//
//	|          | Normal | JSON   |
//	|----------|--------|--------|
//	| output   | stdout | stdout |
//	| errors   | stderr | stdout |
//	| logs     | stderr | stderr |
//	| progress | stderr | stdout |
type Output struct {
	Stdout   io.Writer
	Stderr   io.Writer
	Logger   *slog.Logger
	Option   OutputOpt
	progress *progressBar
}

func NewOutput(opt OutputOpt) *Output {
	return &Output{
		Logger: newCliLogger(opt),
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Option: opt,
	}
}

func (o *Output) Process(cliResult types.Result, err error) int {
	if err == nil {
		return o.processResult(cliResult)
	} else {
		return o.processError(err)
	}
}

func (o *Output) InitProgress(manifest types.ProgressManifest) {
	if o.Option.JSON {
		o.jsonToStd(
			jsonStdout{
				Type: jsonStdoutTypeProgess,
				Progress: &progressPayload{
					Type:     progressTypeManifest,
					Manifest: manifest,
				},
			},
			false,
		)
	} else {
		o.progress = newProgressBar(manifest.Operation, 0, o.Stderr)
		o.progress.StartDisplay()
	}
}

func (o *Output) UpdateProgress(updatedItem types.ProgressItem) {
	if o.Option.JSON {
		o.jsonToStd(
			jsonStdout{
				Type: jsonStdoutTypeProgess,
				Progress: &progressPayload{
					Type: progressTypeUpdate,
					Item: updatedItem,
				},
			},
			false,
		)
	} else {
		if o.progress != nil {
			o.progress.UpdateItem(updatedItem.Name, updatedItem.Size, updatedItem.Size)
		}
	}
}

func (o *Output) CloseProgress(progressFinish types.ProgressFinish) {
	if o.Option.JSON {
		o.jsonToStd(
			jsonStdout{
				Type: jsonStdoutTypeProgess,
				Progress: &progressPayload{
					Type:   progressTypeFinish,
					Finish: progressFinish,
				},
			},
			false,
		)
	} else {
		if o.progress != nil {
			o.progress.Done()
			// Give the display goroutine a moment to print final message
			time.Sleep(150 * time.Millisecond)
		}
	}
}

func (o *Output) processResult(cliResult types.Result) int {
	if o.Option.JSON {
		o.jsonToStd(
			jsonStdout{
				Type:   jsonStdoutTypeOutput,
				Result: cliResult,
			},
			false,
		)
	} else {
		o.HumanReadableToStd(cliResult.ToStringSlice(), false)
	}
	return vit.ExitSuccess
}

func (o *Output) processError(err error) int {
	o.logError(err)
	if o.Option.JSON {
		o.processErrorJSON(err)
	} else {
		o.processErrorHumanReadable(err)
	}
	return vit.ExitGeneralError
}

func (o *Output) logError(err error) {
	var topErr *types.TopLevelError
	if !errors.As(err, &topErr) {
		return
	}
	o.Logger.Error("failure", "command", "vit."+topErr.CommandName)

	var vitErr *types.VitError
	if errors.As(topErr.NestedErr, &vitErr) {
		o.Logger.Error(vitErr.Name, vitErr.Extra...)
		if vitErr.NestedErr != nil {
			o.Logger.Error("cause", "error", vitErr.NestedErr.Error())
		}
	} else {
		o.Logger.Error("UnexpectedError", "error", topErr.NestedErr.Error())
	}
}

func (o *Output) processErrorJSON(err error) {
	jsonStdErr := jsonStdErr{}
	var topErr *types.TopLevelError
	if errors.As(err, &topErr) {
		var vitErr *types.VitError
		if errors.As(topErr.NestedErr, &vitErr) {
			if vitErr.Internal {
				jsonStdErr.Type = jsonStdErrInternal
			} else {
				jsonStdErr.Type = jsonStdErrStandard
			}
			jsonStdErr.Name = vitErr.Name
			jsonStdErr.Message = vitErr.Message
			jsonStdErr.Extra = vitErr.Extra
			if vitErr.NestedErr != nil {
				jsonStdErr.RawErr = vitErr.NestedErr.Error()
			}
		} else {
			jsonStdErr.Type = jsonStdErrUnexpected
			jsonStdErr.RawErr = topErr.NestedErr.Error()
		}
	} else {
		jsonStdErr.Type = jsonStdErrUnexpected
		jsonStdErr.RawErr = err.Error()
	}

	o.jsonToStd(
		jsonStdout{
			Type:  jsonStdoutTypeError,
			Error: &jsonStdErr,
		},
		false,
	)
}

func (o *Output) processErrorHumanReadable(err error) {
	var topErr *types.TopLevelError
	if errors.As(err, &topErr) {
		var vitErr *types.VitError
		if errors.As(topErr.NestedErr, &vitErr) {
			messages := slices.Concat(vitErr.Message, []string{fmt.Sprintf("[%s]", vitErr.Name)})
			o.HumanReadableToStd(messages, true)
		} else {
			o.HumanReadableToStd([]string{topErr.NestedErr.Error()}, true)
		}
	} else {
		o.HumanReadableToStd([]string{err.Error()}, true)
	}
}

func (o *Output) HumanReadableToStd(lines []string, stderr bool) {
	// if json and debug, standard human readable destined to normal user are disabled.
	// -> no pollution if we are streaming json.
	// -> nor if we want clean debug (structured) logs.
	if o.Option.Debug || o.Option.JSON {
		return
	}
	var std io.Writer
	if stderr {
		std = o.Stderr
	} else {
		std = o.Stdout
	}

	for _, line := range lines {
		fmt.Fprint(std, line+"\n")
	}
}

func (o *Output) jsonToStd(jsonStdout jsonStdout, stderr bool) {
	jsonData, err := json.Marshal(jsonStdout)
	if err != nil {
		o.Logger.Error("failed to marshal JSON output", "error", err)
		return
	}
	var std io.Writer
	if stderr {
		std = o.Stderr
	} else {
		std = o.Stdout
	}
	fmt.Fprintln(std, string(jsonData))
}
