package clicore

import (
	"vit/internal/types"
)

type OutputOpt struct {
	Verbose bool
	Debug   bool
	JSON    bool
}

// JSON related data  --------------------------------------------------------

type jsonStdoutType string

const (
	jsonStdoutTypeOutput  jsonStdoutType = "output"
	jsonStdoutTypeError   jsonStdoutType = "error"
	jsonStdoutTypeProgess jsonStdoutType = "progress"
)

type jsonStdErrType string

const (
	jsonStdErrStandard   jsonStdErrType = "standard"
	jsonStdErrInternal   jsonStdErrType = "internal"
	jsonStdErrUnexpected jsonStdErrType = "unexpected"
)

type jsonStdErr struct {
	Type    jsonStdErrType `json:"type"`
	Name    string         `json:"name"`
	Message []string       `json:"message"`
	Extra   []any          `json:"extra"`
	RawErr  string         `json:"raw_err"`
}

type jsonStdout struct {
	Type     jsonStdoutType `json:"type"`
	Result   types.Result      `json:"result,omitempty"`
	Error    *jsonStdErr    `json:"error,omitempty"`
	Progress *progressPayload `json:"progress,omitempty"`
}

// Progress related data  ----------------------------------------------------

type progressType string

const (
	progressTypeManifest progressType = "manifest"
	progressTypeUpdate   progressType = "update"
	progressTypeFinish   progressType = "finish"
)

type progressPayload struct {
	Type     progressType           `json:"type"`
	Manifest types.ProgressManifest `json:"manifest"`
	Item     types.ProgressItem     `json:"item"`
	Finish   types.ProgressFinish   `json:"finish"`
}

