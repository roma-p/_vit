package types

type FileChangeType string

const (
	FileChangeAdded   FileChangeType = "added"
	FileChangeRemoved FileChangeType = "removed"
	FileChangeEdited  FileChangeType = "edited"
)

type FileChange struct {
	Path string         `json:"path"`
	Type FileChangeType `json:"type"`

	Size int64 `json:"size"` // not filled when ChangeType == Removed
}

type ChangeRecord struct {
	Files        []FileChange `json:"files"`
	Dependencies []any        `json:"dependencies"` // change this type when dependency management added
}

func (c *ChangeRecord) AddChange(path string, changeType FileChangeType, size int64) {
	c.Files = append(c.Files, FileChange{
		Path: path,
		Type: changeType,
		Size: size,
	})
}
