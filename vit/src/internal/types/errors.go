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

func NewTopLevelErrorWith(
	commandName string,
	errorName string,
	errorMessage []string,
	extra []any,
) error {
	return NewTopLevelError(
		commandName,
		NewVitError(errorName, errorMessage, extra),
	)
}

// VitError represents a structured error — both expected (standard) and
// unexpected (internal) conditions. When Internal is true, NestedErr carries
// the underlying Go error.
type VitError struct {
	Internal  bool
	Name      string
	Message   []string // -> will go to stderr as human readable message.
	Extra     []any    // -> will be used as key/value to slog structured logging.
	NestedErr error    // non-nil for internal errors
}

func NewVitError(name string, message []string, extra []any) *VitError {
	return &VitError{
		Name:    name,
		Message: message,
		Extra:   extra,
	}
}

func NewInternalVitError(name string, nestedErr error, message []string, extra []any) *VitError {
	return &VitError{
		Internal:  true,
		Name:      name,
		Message:   message,
		Extra:     extra,
		NestedErr: nestedErr,
	}
}

func (e *VitError) Error() string {
	s := fmt.Sprintf("error %s %s %s",
		e.Name,
		linearizeStringSlice(e.Message),
		linearizeAndStringifyAnySlice(e.Extra),
	)
	if e.NestedErr != nil {
		s += fmt.Sprintf(" cause: %s", e.NestedErr.Error())
	}
	return s
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
