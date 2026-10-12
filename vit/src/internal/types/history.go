package types

import "time"

type HistoryEntry struct {
	UID       string     `json:"uid"`
	Command   VitCommand `json:"command"`
	AssetPath string     `json:"asset_path,omitempty"`
	Ref       *Ref       `json:"ref,omitempty"`
	Message   string     `json:"message,omitempty"`
	User      string     `json:"user"`
	Thumbnail string     `json:"thumbnail,omitempty"`
	Timestamp time.Time  `json:"timestamp"`

	Details string `json:"details,omitempty"` // human readable description of each operation
}
