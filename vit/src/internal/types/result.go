package types

import "fmt"

// Result types
// These structs define the JSON structure returned by each command
// and the strings returned by the cli (using the ToStringSlice() method)
// and are used to automatically generate Python TypedDict definitions
type Result interface {
	ToStringSlice() []string
}

// EmptyResult is returned by commands that don't produce output -------------
type EmptyResult struct{}

func (r *EmptyResult) ToStringSlice() []string {
	return []string{}
}

// StringResult return a simple string ---------------------------------------
type StringResult struct {
	String string `json:"string"`
}

func (r *StringResult) ToStringSlice() []string {
	return []string{r.String}
}

// StringListResult ----------------------------------------------------------
type StringListResult struct {
	StringList []string `json:"string_list"`
}

func (r *StringListResult) ToStringSlice() []string {
	return r.StringList
}

// StatusResult --------------------------------------------------------------

type StatusResult ChangeRecord

func (c *StatusResult) ToStringSlice() []string {

	if len(c.Files) == 0 {
		return []string{"Branch clean, no change to commit"}
	}

	var ret []string

	var added []string
	var edited []string
	var removed []string

	var sizeAdded int64
	var sizeRemoved int64
	for _, f := range c.Files {
		switch f.Type {
		case FileChangeAdded:
			added = append(added, f.Path)
			sizeAdded += f.Size
		case FileChangeRemoved:
			removed = append(removed, f.Path)
			sizeRemoved += f.Size
		case FileChangeEdited:
			edited = append(edited, f.Path)
			sizeAdded += f.Size
		default:
			panic("unreachable")
		}
	}

	ret = append(ret, "* added:")
	for _, f := range added {
		ret = append(ret, f)
	}
	ret = append(ret, "")
	ret = append(ret, "* removed:")
	for _, f := range removed {
		ret = append(ret, f)
	}
	ret = append(ret, "")
	ret = append(ret, "* edited:")
	for _, f := range edited {
		ret = append(ret, f)
	}

	ret = append(ret, "")
	ret = append(ret, fmt.Sprintf("+ %d  - %d", sizeAdded, sizeRemoved))
	
	// TODO: formatter for size.
	// TODO: formatter for sequence!

	return ret
}
