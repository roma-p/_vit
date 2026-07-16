package types

type FileChangeType string

const (
	FileChangeAdded FileChangeType = "added"
	FileChangeRemoved = "removed"
	FileChangeEdited = "edited"
)

type FileChange struct {
	Path string `json:"path"`
	Type FileChangeType `json:"type"`

	Size int64 `json:"size"`// not filled when ChangeType == Removed
}

type ChangeRecord struct {
	Files []FileChange `json:"files"`
	Dependancies []any `json:"dependancies"` // change this type when dependancy management added
}

func (c *ChangeRecord) AddChange(path string, changeType FileChangeType, size int64) {
	c.Files = append(c.Files, FileChange{
		Path: path,
		Type: changeType,
		Size: size,
	})
}
