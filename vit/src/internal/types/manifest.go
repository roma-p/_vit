package types

import "time"

// Manifest is the sidecar describing a commit blob object.
// it is only meant as an escape hatch for vit.
type Manifest struct {
	Timestamp time.Time         `json:"timestamp"`
	Ref       *Ref              `json:"ref"`
	User      string            `json:"user"`
	Sources   map[string]string `json:"sources,omitempty"`
	Message   string            `json:"message,omitempty"`
	// Dependencies []Dependency `json:"dependencies,omitempty"` // TODO: when dependency mgmt is done
}
