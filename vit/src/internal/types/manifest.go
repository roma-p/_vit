package types

import "time"

// Manifest is the sidecar describing a commit blob object.
type Manifest struct {
	Timestamp time.Time         `json:"timestamp"`
	Ref       *Ref              `json:"ref"`
	User      string            `json:"user"`
	Sources   map[string]string `json:"sources,omitempty"`
	// Dependencies []Dependency `json:"dependencies,omitempty"` // TODO: when dependency mgmt is done
}
