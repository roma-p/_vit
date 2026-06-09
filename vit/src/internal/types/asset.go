package types

import (
	"time"
)

type Asset struct {
	AssetUID string                          `json:"asset-uid"`
	Commits  map[string]*AssetCommit         `json:"commits"`
	Branches map[string]*AssetCommit         `json:"branches"`
	Tags     map[string]map[string]*AssetTag `json:"tags,omitempty"`

	AssetPath string `json:"-"` // set at load time, not persisted
}

type AssetCommit struct {
	PayloadSize  int64                       `json:"size"`
	PayloadFile  string                      `json:"file"`
	PayloadHash  string                      `json:"hash"`
	Author       string                      `json:"author"`
	Timestamp    time.Time                   `json:"timestamp"`
	Message      string                      `json:"message,omitempty"`
	Version      int                         `json:"version"`
	Parent       string                      `json:"parent,omitempty"`
	Dependencies map[string]*AssetDependency `json:"dependencies,omitempty"`
}

type AssetDependency struct {
	Type   *AssetDependencyType `json:"type"`
	Ref    *Ref                 `json:"ref"`
	Commit string               `json:"commit"`
}

type AssetDependencyType string

const (
	AssetDependencySource AssetDependencyType = "source"
	AssetDependencyResult AssetDependencyType = "result"
)

type AssetTag struct {
	Message   string    `json:"message,omitempty"`
	Author    string    `json:"author"`
	Commit    string    `json:"commit"`
	Timestamp time.Time `json:"timestamp"`
}
