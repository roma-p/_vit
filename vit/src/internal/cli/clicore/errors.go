
package clicore

import (
	"fmt"
)

type UsageErrorType string

const (
	ErrUnknownSubCommand       UsageErrorType = "UnknownSubcommand"
	ErrNotEnoughPositionalArgs UsageErrorType = "PosArgsNotEnough"
	ErrTooMuchPositionalArgs   UsageErrorType = "PosArgsTooMuch"
	ErrInvalidFlags            UsageErrorType = "InvalidFlags"
)

type UsageError struct {
	Command     string
	CommandName string
	ErrorType   UsageErrorType
	NestedErr   error
	Message     string
}

func NewUsageError(name string, args []string, errType UsageErrorType, nestedErr error) *UsageError {
	ret := UsageError{
		CommandName: name,
		Command:     rebuildCmdFromArgs(name, args),
		ErrorType:   errType,
		NestedErr:   nestedErr,
	}
	messageRoot := "invalid command: "
	switch errType {
	case ErrTooMuchPositionalArgs:
		ret.Message = messageRoot + "too much positional arguments"
	case ErrNotEnoughPositionalArgs:
		ret.Message = messageRoot + "not enough positional arguments"
	case ErrUnknownSubCommand:
		ret.Message = messageRoot + "unknown subcommands"
	case ErrInvalidFlags:
		ret.Message = fmt.Sprintf("%s %s", messageRoot, ret.NestedErr.Error())
	}
	return &ret
}

func (e *UsageError) Error() string {
	return fmt.Sprintf("usage error: %s", e.Command)
}

func rebuildCmdFromArgs(commandName string, args []string) string {
	ret := ""
	ret += commandName
	for _, arg := range args {
		ret += " " + arg
	}
	return ret
}
