package types

import "fmt"

// TopLevelError wraps command-level errors with context about which command failed.
type TopLevelError struct {
	CommandName string
	NestedErr   error
}

func NewTopLevelError(commandName string, nestedErr error) *TopLevelError {
	return &TopLevelError{
		CommandName: commandName,
		NestedErr:   nestedErr,
	}
}

func (e *TopLevelError) Error() string {
	return fmt.Sprintf(
		"error %s:: %s",
		e.CommandName,
		e.NestedErr.Error(),
	)
}

func NewTopLevelErrorWithStandardError(
	commandName string,
	errorName string,
	errorMessage []string,
	extra []any,
) error {
	return NewTopLevelError(
		commandName,
		NewStandardError(errorName, errorMessage, extra),
	)
}

func NewTopLevelErrorWithInternalError(commandName string, err error, message string) error {
	return NewTopLevelError(
		commandName,
		NewInternalError(err, message),
	)
}

// StandardError represents an expected error condition with structured information.
type StandardError struct {
	Name    string
	Message []string // -> will go to stderr as human readable message.
	Extra   []any    // -> will be used as key/value to slog structured logging.
}

func NewStandardError(name string, message []string, extra []any) *StandardError {
	return &StandardError{
		Name:    name,
		Message: message,
		Extra:   extra,
	}
}

func (e *StandardError) Error() string {
	return fmt.Sprintf(
		"error %s %s %s",
		e.Name,
		linearizeStringSlice(e.Message),
		linearizeAndStringifyAnySlice(e.Extra),
	)
}

// InternalError is a thin wrapper around unexpected errors we don't handle specifically.
type InternalError struct {
	NestedErr error
	Message   string
}

func NewInternalError(nestedErr error, message string) *InternalError {
	return &InternalError{
		NestedErr: nestedErr,
		Message:   message,
	}
}

func (e *InternalError) Error() string {
	return e.NestedErr.Error()
}

func linearizeStringSlice(messages []string) string {
	ret := ""
	for _, v := range messages {
		ret += " " + v
	}
	return ret
}

func linearizeAndStringifyAnySlice(values []any) string {
	ret := ""
	for _, v := range values {
		ret += fmt.Sprintf(" %#v", v)
	}
	return ret
}

type CancelledError struct {
	NestedErr error
}

func NewCancelledError(nestedErr error) *CancelledError {
	return &CancelledError{NestedErr: nestedErr}
}

func (e *CancelledError) Error() string {
	return "Operation cancelled: " + e.NestedErr.Error()
}

