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

type JSONStdoutType string

const (
	JSONStdoutTypeOutput  JSONStdoutType = "output"
	JSONStdoutTypeError   JSONStdoutType = "error"
	JSONStdoutTypeProgess JSONStdoutType = "progress"
)

type JSONStdErrType string

const (
	JSONStdErrStandard   JSONStdErrType = "standard"
	JSONStdErrInternal   JSONStdErrType = "internal"
	JSONStdErrUnexpected JSONStdErrType = "unexpected"
)

type JSONStdErr struct {
	Type    JSONStdErrType `json:"type"`
	Name    string         `json:"name"`
	Message []string       `json:"message"`
	Extra   []any          `json:"extra"`
	RawErr  string         `json:"raw_err"`
}

type JSONStdout struct {
	Type     JSONStdoutType `json:"type"`
	Result   types.Result      `json:"result,omitempty"`
	Error    *JSONStdErr    `json:"error,omitempty"`
	Progress *Progress      `json:"progress,omitempty"`
}

// Progress related data  ----------------------------------------------------

type ProgressType string

const (
	ProgressTypeManifest ProgressType = "manifest"
	ProgressTypeUpdate   ProgressType = "update"
	ProgressTypeFinish   ProgressType = "finish"
)

type Progress struct {
	Type     ProgressType           `json:"type"`
	Manifest types.ProgressManifest `json:"manifest"`
	Item     types.ProgressItem     `json:"item"`
	Finish   types.ProgressFinish   `json:"finish"`
}

