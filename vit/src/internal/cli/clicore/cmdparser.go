package clicore

import (
	"bytes"
	"flag"
	"fmt"
	"slices"
	"strings"
)

type flagDef struct {
	flagType   string // "bool", "string", etc.
	defaultVal any
	usage      string
}

type CmdParser struct {
	Name            string
	FlagSet         *flag.FlagSet
	PosArgs         []string
	OptionalPosArgs []string
	FlagArgs        map[string]any
	flagDefs        map[string]flagDef // Store flag definitions for resetting
	ArgsMap         map[string]string
	Usage           []string
}

func (a *CmdParser) Bool(name string, value bool, usage string) error {
	if _, ok := a.FlagArgs[name]; ok {
		return fmt.Errorf("already a flag '%s' in parser '%s'", name, a.Name)
	}
	a.flagDefs[name] = flagDef{flagType: "bool", defaultVal: value, usage: usage}
	a.FlagArgs[name] = a.FlagSet.Bool(name, value, usage)
	return nil
}

// TODO: does not work.
func (a *CmdParser) String(name string, value string, usage string) error {
	if _, ok := a.FlagArgs[name]; ok {
		return fmt.Errorf("already a flag '%s' in parser '%s'", name, a.Name)
	}
	a.flagDefs[name] = flagDef{flagType: "string", defaultVal: value, usage: usage}
	a.FlagArgs[name] = a.FlagSet.String(name, value, usage)
	return nil
}

func (a *CmdParser) GetFlag(name string) any {
	return a.FlagArgs[name]
}

func (a *CmdParser) GetArg(name string) string {
	return a.ArgsMap[name]
}

func NewArgParser(name string, posArgs []string, optArgs []string) *CmdParser {
	ret := CmdParser{
		Name:            name,
		FlagSet:         flag.NewFlagSet(name, flag.ContinueOnError),
		PosArgs:         posArgs,
		OptionalPosArgs: optArgs,
		FlagArgs:        make(map[string]any),
		flagDefs:        make(map[string]flagDef),
	}

	_ = ret.Bool("json", false, "Enable JSON output format")
	_ = ret.Bool("debug", false, "Enable debug logging")
	_ = ret.Bool("v", false, "Enable verbose logging")
	_ = ret.Bool("h", false, "help")
	// ignoring err: only way to have err is to have duplicated flag.
	// which we know that's not the case here.

	ret.buildUsage()
	return &ret
}

func (a *CmdParser) resetFlagSet() {
	a.FlagSet = flag.NewFlagSet(a.Name, flag.ContinueOnError)
	for name, def := range a.flagDefs {
		switch def.flagType {
		case "bool":
			a.FlagArgs[name] = a.FlagSet.Bool(name, def.defaultVal.(bool), def.usage)
		case "string":
			a.FlagArgs[name] = a.FlagSet.String(name, def.defaultVal.(string), def.usage)
		}
	}
}

func (a *CmdParser) Parse(args []string) *UsageError {
	a.resetFlagSet()

	posArgs, flagArgs := splitPosFlagArgs(args)

	// Parse flags FIRST so they're available even if there's a usage error
	if err := a.FlagSet.Parse(flagArgs); err != nil {
		return NewUsageError(a.Name, args, errInvalidFlags, err)
	}

	// Calculate total possible positional args (required + optional)
	totalPosArgs := len(a.PosArgs) + len(a.OptionalPosArgs)

	// Validate positional args
	if len(posArgs) < len(a.PosArgs) {
		return NewUsageError(a.Name, args, errNotEnoughPositionalArgs, nil)
	} else if len(posArgs) > totalPosArgs {
		return NewUsageError(a.Name, args, errTooMuchPositionalArgs, nil)
	}

	// Map positional args (required + optional)
	a.ArgsMap = make(map[string]string)
	allPosArgNames := slices.Concat(a.PosArgs, a.OptionalPosArgs)
	for i, argName := range allPosArgNames {
		if i < len(posArgs) {
			a.ArgsMap[argName] = posArgs[i]
		}
	}
	return nil
}

func (a *CmdParser) flagDefaultsString() []string {
	var buf bytes.Buffer
	originalOutput := a.FlagSet.Output()
	a.FlagSet.SetOutput(&buf)
	a.FlagSet.PrintDefaults()
	a.FlagSet.SetOutput(originalOutput)
	return strings.Split(strings.TrimSpace(buf.String()), "\n")
}

func (a *CmdParser) buildUsage() {
	var u []string
	usageFirstLine := fmt.Sprintf("Usage: %s", a.Name)
	for _, arg := range a.PosArgs {
		usageFirstLine = fmt.Sprintf("%s <%s>", usageFirstLine, arg)
	}
	for _, arg := range a.OptionalPosArgs {
		usageFirstLine += fmt.Sprintf(" [%s]", arg)
	}
	usageFirstLine += " [flags]\n"
	u = append(u, usageFirstLine)
	u = append(u, "Flags:")
	u = append(u, a.flagDefaultsString()...)
	a.Usage = u
}

func splitPosFlagArgs(args []string) ([]string, []string) {
	var flagArgs []string
	var posArgs []string

	for _, arg := range args {
		if len(arg) > 0 && arg[0] == '-' {
			flagArgs = append(flagArgs, arg)
		} else {
			posArgs = append(posArgs, arg)
		}
	}
	return posArgs, flagArgs
}
