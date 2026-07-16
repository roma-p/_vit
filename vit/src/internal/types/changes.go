package types

type ChangeType uint8

const (
	Added ChangeType = iota
	Removed
	Edited
)

type Change struct {
	Path string
	Type ChangeType

	Size int64 // not filled when ChangeType == Removed
}

type Changes []Change

func (c *Changes) AddChange(path string, changeType ChangeType, size int64) {
	*c = append(*c, Change{
		Path: path,
		Type: changeType,
		Size: size,
	})
}

func (c *Changes) ToStringSlice() []string {
	var ret []string

	var added []string
	var deleted []string
	var removed []string

	for f, c := range c {
		if c.Type
	}
}
