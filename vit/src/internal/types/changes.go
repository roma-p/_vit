package types

type ChangeRecord struct {
	Files        []FileChange `json:"files"`
	Dependencies []any        `json:"dependencies"` // change this type when dependency management added
}

type FileChange struct {
	Path string         `json:"path"`
	Type FileChangeType `json:"type"`

	Size int64 `json:"size"` // not filled when ChangeType == Removed
}

type FileChangeType string

const (
	FileChangeAdded   FileChangeType = "added"
	FileChangeRemoved FileChangeType = "removed"
	FileChangeEdited  FileChangeType = "edited"
)

func (c *ChangeRecord) AddChange(path string, changeType FileChangeType, size int64) {
	c.Files = append(c.Files, FileChange{
		Path: path,
		Type: changeType,
		Size: size,
	})
}

func (c *ChangeRecord) HasChange() bool {
	return len(c.Files) != 0 || len(c.Dependencies) != 0
}
