package clicore

import (
	"errors"
	"testing"
)

func TestUsageErrorMessageCarriesTheCause(t *testing.T) {
	cause := errors.New(`expected key=value, got "seq"`)
	for _, typ := range []UsageErrorType{ErrInvalidCommand, errInvalidFlags} {
		if got := NewUsageError("query", nil, typ, cause).Message; got != `invalid command: expected key=value, got "seq"` {
			t.Errorf("%s: message = %q", typ, got)
		}
	}
}
